package service

import (
	"context"
	appconfig "ctoup.com/coreapp/pkg/shared/config"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cloud.google.com/go/storage" // GCS client
	"ctoup.com/coreapp/pkg/shared/util"

	// AWS S3 client imports
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/rs/zerolog/log"

	// Azure Blob client imports
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"

	"github.com/gin-gonic/gin"
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
	cfg := appconfig.StorageSettings()
	provider, bucketName := cfg.Provider, cfg.Bucket

	if provider != "" && provider != "file" && bucketName == "" {
		log.Error().Str("provider", provider).Msg("storage: no bucket configured for this provider")
	}

	// Creating the bucket is now OPT-IN. It used to run unconditionally, from
	// two handler constructors — so a deployment whose storage is provisioned
	// by terraform made two doomed round trips and logged two permission errors
	// at every boot, and AZURE_STORAGE_KEY / GOOGLE_APPLICATION_CREDENTIALS
	// existed solely to service them.
	if cfg.BootstrapBucket && bucketName != "" {
		var err error
		switch provider {
		case "gcs":
			err = createGCSBucketIfNotExists(context.Background(), bucketName)
		case "s3":
			err = createS3BucketIfNotExists(context.Background(), bucketName)
		case "azure":
			err = createAzureContainerIfNotExists(context.Background(), bucketName)
		}
		if err != nil {
			log.Err(err).Str("provider", provider).Msg("storage: could not ensure the bucket exists")
		}
	}

	// Construct the bucket URL based on the provider and bucket name.
	var bucketURL string
	switch provider {
	case "gcs":
		bucketURL = "gs://" + bucketName
	case "s3":
		bucketURL = "s3://" + bucketName + "?region=" + cfg.AWSRegion
	case "azure":
		bucketURL = "azblob://" + bucketName
	case "file":
		fallthrough
	default:
		bucketURL = cfg.LocalPath
	}

	// Open the bucket once and store the client
	b, err := blob.OpenBucket(context.Background(), bucketURL)
	if err != nil {
		log.Err(err).Msgf("Failed to open bucket at URL: %s", bucketURL)
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
	// Google's own ADC convention, deliberately left as an environment read.
	// The blob client resolves ADC by itself; this only parses the same file for
	// its project_id. Routing it through Config would mean this library
	// re-implementing credential discovery its SDK already does — and every
	// other tool in the Google ecosystem reads this variable the same way.
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

	// `*types.NotFound` is what implements error, not `types.NotFound`, so the
	// value form made errors.As PANIC on every HeadBucket failure. `go vet`
	// catches this, but it had never run here — the package had no test file.
	//
	// NOTE: the condition below also reads inverted (it creates the bucket when
	// the error is NOT NotFound). Left as it was deliberately: this path needs a
	// real S3 account to exercise, and quietly changing bucket provisioning is
	// not something to bundle into a range-request fix. Worth its own change.
	var apiError *types.NotFound
	if !errors.As(err, &apiError) {
		log.Info().Msgf("S3 bucket not found, creating it now.")
		_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(bucketName),
			CreateBucketConfiguration: &types.CreateBucketConfiguration{
				LocationConstraint: types.BucketLocationConstraint(appconfig.StorageSettings().AWSRegion),
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
	cfg := appconfig.StorageSettings()
	accountName, accountKey := cfg.AzureAccount, cfg.AzureKey

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
	logger := util.GetLoggerFromCtx(ctx)
	// We can now use the `fs.bucket` attribute directly.
	w, err := fs.bucket.NewWriter(ctx, filename, nil)
	if err != nil {
		logger.Err(err).Msg("Failed to create new writer")
		return err
	}

	// Write the data to the file.
	if _, err = w.Write(data); err != nil {
		logger.Err(err).Msg("Failed to write data to file")
		w.Close() // Ensure the writer is closed even on error.
		return err
	}

	// Close the writer to finalize the write operation.
	return w.Close()
}

// DeleteFile deletes a file from the specified bucket.
func (fs *FileService) DeleteFile(ctx context.Context, filename string) error {
	logger := util.GetLoggerFromCtx(ctx)
	// We can now use the `fs.bucket` attribute directly.
	if err := fs.bucket.Delete(ctx, filename); err != nil {
		logger.Err(err).Msgf("Failed to delete file %s", filename)
		return err
	}
	return nil
}

// CopyFile copies a file from src to dst within the same bucket.
func (fs *FileService) CopyFile(ctx context.Context, dst, src string) error {
	logger := util.GetLoggerFromCtx(ctx)
	if err := fs.bucket.Copy(ctx, dst, src, nil); err != nil {
		logger.Err(err).Msgf("Failed to copy file from %s to %s", src, dst)
		return err
	}
	return nil
}

// RenameFile moves a file from src to dst by copying then deleting.
func (fs *FileService) RenameFile(ctx context.Context, dst, src string) error {
	logger := util.GetLoggerFromCtx(ctx)
	if err := fs.CopyFile(ctx, dst, src); err != nil {
		logger.Err(err).Msgf("Failed to copy file from %s to %s during rename", src, dst)
		return err
	}
	return fs.DeleteFile(ctx, src)
}

// FileExists checks if a file exists in the bucket.
func (fs *FileService) FileExists(ctx context.Context, filename string) (bool, error) {
	logger := util.GetLoggerFromCtx(ctx)
	exists, err := fs.bucket.Exists(ctx, filename)
	if err != nil {
		logger.Err(err).Msgf("Failed to check existence of file %s", filename)
		return false, err
	}
	return exists, nil
}

func (fs *FileService) ReadFileBytes(ctx context.Context, filename string) ([]byte, error) {
	reader, err := fs.bucket.NewReader(ctx, filename, nil)
	if err != nil {
		return nil, fmt.Errorf("open file %s: %w", filename, err)
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// GetFile streams a file from the bucket to the HTTP response, with ETag
// revalidation and byte-range support.
//
// Ranges matter for more than seeking convenience. A browser will not let a
// user scrub a <video>/<audio> element at all unless the origin answers a
// ranged request with 206 — a 200 produces media that plays from the start and
// cannot be sought, with nothing in the console to say why. PDF.js and any
// resumable download need the same.
//
// It also streams rather than buffering. The previous implementation read the
// whole object into memory to hash it for the ETag, so serving one 150 MB
// lesson video cost 150 MB of heap per concurrent request.
func (fs *FileService) GetFile(ctx *gin.Context, filename string) error {
	logger := util.GetLoggerFromCtx(ctx)

	attrs, err := fs.bucket.Attributes(ctx, filename)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			logger.Warn().Msgf("Failed to find file %s", filename)
			ctx.AbortWithStatus(http.StatusNotFound)
		} else {
			logger.Err(err).Msgf("Failed to stat file %s", filename)
			ctx.AbortWithError(http.StatusInternalServerError, err)
		}
		return err
	}

	etag := etagFor(attrs)

	ctx.Header("ETag", etag)
	ctx.Header("Content-Type", fs.getContentType(filename))
	ctx.Header("Cache-Control", "public, max-age=3600")
	// Advertised unconditionally: a client that cannot see Accept-Ranges will
	// not attempt a ranged request in the first place.
	ctx.Header("Accept-Ranges", "bytes")
	ctx.Header("Vary", "Origin, Authorization")
	ctx.Header("Access-Control-Expose-Headers", "ETag, Cache-Control, Accept-Ranges, Content-Range")

	// Even when the client sends Cache-Control: no-cache we still honour the
	// validator, so conditional requests work as intended.
	if clientETag := ctx.GetHeader("If-None-Match"); clientETag != "" {
		if strings.Trim(clientETag, `"`) == strings.Trim(etag, `"`) {
			ctx.Header("Cache-Control", "public, max-age=3600")
			ctx.Status(http.StatusNotModified)
			return nil
		}
	}

	offset, length, status, err := resolveRange(ctx.GetHeader("Range"), attrs.Size)
	if err != nil {
		// RFC 9110: an unsatisfiable range is answered with the object's real
		// size so the client can correct itself rather than guess.
		ctx.Header("Content-Range", fmt.Sprintf("bytes */%d", attrs.Size))
		ctx.AbortWithStatus(http.StatusRequestedRangeNotSatisfiable)
		return err
	}

	if status == http.StatusPartialContent {
		ctx.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, attrs.Size))
	}
	ctx.Header("Content-Length", fmt.Sprintf("%d", length))

	// length -1 means "to the end" for NewRangeReader, but we have already
	// resolved it to a concrete count so Content-Length can be exact.
	reader, err := fs.bucket.NewRangeReader(ctx, filename, offset, length, nil)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			ctx.AbortWithStatus(http.StatusNotFound)
		} else {
			logger.Err(err).Msgf("Failed to open file %s", filename)
			ctx.AbortWithError(http.StatusInternalServerError, err)
		}
		return err
	}
	defer reader.Close()

	ctx.Status(status)
	if _, err := io.Copy(ctx.Writer, reader); err != nil {
		// The status and headers are already on the wire, so there is no way to
		// turn this into an error response — the client sees a truncated body.
		// Log it and return; calling AbortWithError here would only add a
		// spurious second status.
		logger.Err(err).Msgf("Failed while streaming file %s", filename)
		return err
	}

	return nil
}

// etagFor derives a cache validator from object metadata rather than from the
// bytes, so the whole object never has to be read to serve a conditional
// request.
//
// Where the driver reports a content MD5 (S3, GCS) this reproduces the value
// the previous content-hashing implementation produced, so existing caches stay
// valid. Where it does not (fileblob), size and mod-time identify the version
// well enough for a validator.
func etagFor(attrs *blob.Attributes) string {
	if len(attrs.MD5) > 0 {
		return fmt.Sprintf(`"%x"`, attrs.MD5)
	}
	return fmt.Sprintf(`"%d-%d"`, attrs.Size, attrs.ModTime.UnixNano())
}

// errUnsatisfiableRange is returned for a syntactically valid Range header that
// cannot be served against this object's size.
var errUnsatisfiableRange = errors.New("requested range not satisfiable")

// resolveRange turns a Range header into a concrete (offset, length) pair.
//
// Returns 200 and the whole object when the header is absent or is not a form
// we serve; a malformed header is deliberately IGNORED rather than rejected, as
// RFC 9110 requires — refusing it would break clients that send something
// unexpected but would be perfectly happy with the entire body.
//
// Only single ranges are supported. Multipart/byteranges buys nothing for media
// playback and doubles the surface area.
func resolveRange(header string, size int64) (offset, length int64, status int, err error) {
	const prefix = "bytes="
	spec, ok := strings.CutPrefix(strings.TrimSpace(header), prefix)
	if header == "" || !ok || strings.Contains(spec, ",") {
		return 0, size, http.StatusOK, nil
	}

	start, end, ok := strings.Cut(spec, "-")
	if !ok {
		return 0, size, http.StatusOK, nil
	}
	start, end = strings.TrimSpace(start), strings.TrimSpace(end)

	switch {
	case start == "" && end == "":
		return 0, size, http.StatusOK, nil

	case start == "":
		// Suffix form, "bytes=-N": the LAST n bytes. Used by PDF readers to
		// find a trailer without fetching the file.
		n, convErr := strconv.ParseInt(end, 10, 64)
		if convErr != nil {
			return 0, size, http.StatusOK, nil
		}
		if n <= 0 {
			return 0, 0, 0, errUnsatisfiableRange
		}
		if n > size {
			n = size
		}
		return size - n, n, http.StatusPartialContent, nil

	default:
		from, convErr := strconv.ParseInt(start, 10, 64)
		if convErr != nil || from < 0 {
			return 0, size, http.StatusOK, nil
		}
		// A start at or past the end is unsatisfiable — the one case a client
		// must be told about, since it means its idea of the size is wrong.
		if from >= size {
			return 0, 0, 0, errUnsatisfiableRange
		}

		to := size - 1 // open-ended "bytes=N-"
		if end != "" {
			parsed, convErr := strconv.ParseInt(end, 10, 64)
			if convErr != nil {
				return 0, size, http.StatusOK, nil
			}
			if parsed < from {
				return 0, 0, 0, errUnsatisfiableRange
			}
			if parsed < to {
				to = parsed
			}
		}
		return from, to - from + 1, http.StatusPartialContent, nil
	}
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
