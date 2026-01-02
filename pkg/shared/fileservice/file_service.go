package service

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage" // GCS client

	// AWS S3 client imports
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	// Azure Blob client imports
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
	"google.golang.org/api/option" // Import the option package

	// Import the blob packages we want to be able to open.
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
)

type FileService struct {
	bucket *blob.Bucket
}

func NewFileService() *FileService {
	provider := os.Getenv("FILE_STORAGE_PROVIDER")
	var bucketName string
	switch provider {
	case "gcs":
		bucketName = os.Getenv("GCS_BUCKET_NAME")
		if bucketName == "" {
			log.Error().Msg("GCS_BUCKET_NAME environment variable not set.")
		}
		err := createGCSBucketIfNotExists(context.Background(), bucketName)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create GCS bucket")
		}
	case "s3":
		bucketName = os.Getenv("S3_BUCKET_NAME")
		if bucketName == "" {
			log.Error().Msg("S3_BUCKET_NAME environment variable not set.")
		}
		err := createS3BucketIfNotExists(context.Background(), bucketName)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create S3 bucket")
		}
	case "azure":
		bucketName = os.Getenv("AZURE_STORAGE_CONTAINER_NAME")
		if bucketName == "" {
			log.Error().Msg("AZURE_STORAGE_CONTAINER_NAME environment variable not set.")
		}
		err := createAzureContainerIfNotExists(context.Background(), bucketName)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create Azure container")
		}
	case "file":
	}

	// Construct the bucket URL based on the provider and bucket name.
	var bucketURL string
	switch provider {
	case "gcs":
		bucketURL = "gs://" + bucketName
	case "s3":
		bucketURL = "s3://" + bucketName + "?region=" + os.Getenv("AWS_REGION")
	case "azure":
		bucketURL = "azblob://" + bucketName
	case "file":
		fallthrough
	default:
		bucketURL = os.Getenv("FILE_FOLDER_URL")
	}

	// Open the bucket once and store the client
	b, err := blob.OpenBucket(context.Background(), bucketURL)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to open bucket at URL: %s", bucketURL)
	}

	return &FileService{
		bucket: b,
	}
}

// createGCSBucketIfNotExists uses the GCS client to create a bucket if it does not exist.
func createGCSBucketIfNotExists(ctx context.Context, bucketName string) error {
	log.Info().Msgf("Checking for existence of GCS bucket: %s", bucketName)

	var client *storage.Client
	var err error

	// Check for GCS credentials content first (for CI/CD)
	//
	credsPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	projectID := ""
	if credsPath != "" {
		// read the credentials file
		credsJSON, err := os.ReadFile(credsPath)
		if err != nil {
			return err
		}
		// unmarshal the credentials JSON to get the project ID
		creds := struct {
			ProjectID string `json:"project_id"`
		}{}
		err = json.Unmarshal([]byte(credsJSON), &creds)
		if err != nil {
			return err
		}
		projectID = creds.ProjectID

		log.Info().Msg("Using JSON credentials from environment variable.")
		client, err = storage.NewClient(ctx, option.WithCredentialsJSON([]byte(credsJSON)))
		if err != nil {
			return err
		}
	} else {
		// Fall back to ADC (file path)
		log.Info().Msg("Using credentials from file path or ADC.")
		client, err = storage.NewClient(ctx)
	}

	if err != nil {
		return err
	}
	defer client.Close()

	_, err = client.Bucket(bucketName).Attrs(ctx)
	if err == nil {
		log.Info().Msg("GCS bucket already exists.")
		return nil
	}
	if err != storage.ErrBucketNotExist {
		return err
	}

	log.Info().Msg("GCS bucket not found, creating it now.")
	if err := client.Bucket(bucketName).Create(ctx, projectID, nil); err != nil {
		return err
	}
	log.Info().Msg("GCS bucket created successfully.")
	return nil
}

// createS3BucketIfNotExists uses the AWS S3 client to create a bucket if it does not exist.
func createS3BucketIfNotExists(ctx context.Context, bucketName string) error {
	log.Info().Msgf("Checking for existence of S3 bucket: %s", bucketName)

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}

	client := s3.NewFromConfig(cfg)

	// Check if the bucket exists using HeadBucket
	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})

	if err == nil {
		log.Info().Msg("S3 bucket already exists.")
		return nil
	}

	var apiError types.NotFound
	if !errors.As(err, &apiError) {
		log.Info().Msgf("S3 bucket not found, creating it now.")
		_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(bucketName),
			CreateBucketConfiguration: &types.CreateBucketConfiguration{
				LocationConstraint: types.BucketLocationConstraint(os.Getenv("AWS_REGION")),
			},
		})
		if err != nil {
			return err
		}
	}

	log.Info().Msg("S3 bucket created successfully.")
	return nil
}

// createAzureContainerIfNotExists uses the Azure Blob client to create a container if it does not exist.
func createAzureContainerIfNotExists(ctx context.Context, containerName string) error {
	log.Info().Msgf("Checking for existence of Azure container: %s", containerName)

	// Get credentials from environment variables
	accountName := os.Getenv("AZURE_STORAGE_ACCOUNT")
	accountKey := os.Getenv("AZURE_STORAGE_KEY")

	// Create a SharedKeyCredential
	cred, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		return err
	}

	// Create a client for the service
	serviceClient, err := azblob.NewClientWithSharedKeyCredential(
		"https://"+accountName+".blob.core.windows.net/",
		cred, nil)
	if err != nil {
		return err
	}

	// Create a container client
	containerClient := serviceClient.ServiceClient().NewContainerClient(containerName)

	// Create the container. The Azure SDK's `Create` method returns an error if the container already exists.
	_, err = containerClient.Create(ctx, nil)

	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == http.StatusConflict {
			log.Info().Msg("Azure container already exists.")
			return nil
		}
		return err
	}

	log.Info().Msg("Azure container created successfully.")
	return nil
}

// SaveFile writes data to a file in the specified bucket.
func (fs *FileService) SaveFile(ctx context.Context, data []byte, filename string) error {
	// We can now use the `fs.bucket` attribute directly.
	w, err := fs.bucket.NewWriter(ctx, filename, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create new writer")
		return err
	}

	// Write the data to the file.
	if _, err = w.Write(data); err != nil {
		log.Error().Err(err).Msg("Failed to write data to file")
		w.Close() // Ensure the writer is closed even on error.
		return err
	}

	// Close the writer to finalize the write operation.
	return w.Close()
}

// DeleteFile deletes a file from the specified bucket.
func (fs *FileService) DeleteFile(ctx context.Context, filename string) error {
	// We can now use the `fs.bucket` attribute directly.
	if err := fs.bucket.Delete(ctx, filename); err != nil {
		log.Error().Err(err).Msgf("Failed to delete file %s", filename)
		return err
	}
	return nil
}

// GetFile retrieves a file from the specified bucket and writes its contents to the HTTP response.
// It supports ETag-based caching for improved performance.
func (fs *FileService) GetFile(ctx *gin.Context, filename string) error {
	// Create a new reader to the specified file.
	reader, err := fs.bucket.NewReader(ctx, filename, nil)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			ctx.AbortWithStatus(404)
		} else {
			log.Error().Err(err).Msg("Failed to open file")
			ctx.AbortWithError(500, err)
		}
		return err
	}
	defer reader.Close()

	// Read the file content to generate ETag
	content, err := io.ReadAll(reader)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read file content")
		ctx.AbortWithError(500, err)
		return err
	}

	// Generate ETag based on content hash
	etag := fs.generateETag(content)

	// Set content type based on file extension
	contentType := fs.getContentType(filename)

	// Set cache headers
	ctx.Header("ETag", etag)
	ctx.Header("Content-Type", contentType)
	ctx.Header("Cache-Control", "public, max-age=3600") // Cache for 1 hour

	// Add headers to encourage caching for CORS requests
	ctx.Header("Vary", "Origin, Authorization")
	ctx.Header("Access-Control-Expose-Headers", "ETag, Cache-Control")

	// Check if client has cached version
	// Note: Even if client sends Cache-Control: no-cache, we still check ETag
	// This allows conditional requests to work properly
	if clientETag := ctx.GetHeader("If-None-Match"); clientETag != "" {
		// Remove quotes if present
		clientETag = strings.Trim(clientETag, `"`)
		serverETag := strings.Trim(etag, `"`)

		if clientETag == serverETag {
			// Override any no-cache directives for unchanged content
			ctx.Header("Cache-Control", "public, max-age=3600")
			ctx.Status(http.StatusNotModified)
			return nil
		}
	}

	// Write the blob contents to the response.
	if _, err = ctx.Writer.Write(content); err != nil {
		log.Error().Err(err).Msg("Failed to write file to response writer")
		ctx.AbortWithError(500, err)
		return err
	}

	return nil
}

// generateETag creates an ETag based on the file content hash
func (fs *FileService) generateETag(content []byte) string {
	hash := md5.Sum(content)
	return fmt.Sprintf(`"%x"`, hash)
}

// getContentType determines the MIME type based on file extension
func (fs *FileService) getContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	// Handle common image types explicitly
	switch ext {
	case ".webp":
		return "image/webp"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	default:
		// Use mime package for other types
		contentType := mime.TypeByExtension(ext)
		if contentType == "" {
			return "application/octet-stream"
		}
		return contentType
	}
}
