package service

import (
	"context"
	"io"
	"os"

	"github.com/rs/zerolog/log"

	"github.com/gin-gonic/gin"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob"
	"gocloud.dev/gcerrors"

	// Import the blob packages we want to be able to open.
	// _ "gocloud.dev/blob/memblob"
	// _ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/azureblob"
	// _ "gocloud.dev/blob/gcsblob"
	// _ "gocloud.dev/blob/s3blob"
)

// bucketURL "s3blob://my-bucket") will return a *blob.Bucket created using s3blob.OpenBucket.
// https://pkg.go.dev/gocloud.dev/blob/azureblob
// "AZURE_STORAGE_ACCOUNT_NAME" > AZURE_STORAGE_ACCOUNT
// "AZURE_STORAGE_BLOB_ENDPOINT"
// "AZURE_STORAGE_ACCOUNT_KEY" > AZURE_STORAGE_KEY

func GetBucketURL() string {
	// this example uses a shared key to authenticate with Azure Blob Storage
	/* _, ok := os.LookupEnv("AZURE_STORAGE_ACCOUNT")
	if !ok {
		log.Println("AZURE_STORAGE_ACCOUNT could not be found")
		return os.Getenv("FILE_FOLDER_URL")
	}
	_, ok = os.LookupEnv("AZURE_STORAGE_KEY")
	if !ok {
		log.Println("AZURE_STORAGE_KEY could not be found")
		return os.Getenv("FILE_FOLDER_URL")
	}

	_, ok = os.LookupEnv("AZURE_STORAGE_BLOB_ENDPOINT")
	if !ok {
		log.Println("AZURE_STORAGE_BLOB_ENDPOINT could not be found")
		return os.Getenv("FILE_FOLDER_URL")
	} */

	return os.Getenv("FILE_FOLDER_URL")
}

func SaveFile(ctx context.Context, data []byte, filename string) error {
	bucketURL := GetBucketURL()

	b, err := blob.OpenBucket(ctx, bucketURL)
	if err != nil {
		return err
	}
	defer b.Close()

	w, err := b.NewWriter(ctx, filename, nil)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	if err != nil {
		return err
	}

	return w.Close()
}

func DeleteFile(ctx context.Context, filename string) error {
	b, err := blob.OpenBucket(ctx, GetBucketURL())
	if err != nil {
		return err
	}
	defer b.Close()

	if err = b.Delete(ctx, filename); err != nil {
		return err
	}
	return nil
}

func GetFile(ctx *gin.Context, bucketURL string, filename string) error {
	// Open a connection to the bucket.
	b, err := blob.OpenBucket(ctx, bucketURL)
	if err != nil {
		log.Error().Msg("Failed to setup bucket: " + err.Error())
		ctx.AbortWithError(500, err)
		return err
	}
	defer b.Close()

	reader, err := b.NewReader(ctx, filename, nil)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			ctx.AbortWithStatus(404)
			return err
		} else {
			log.Printf("Failed to open file: %s", err)
			ctx.AbortWithError(500, err)
			return err
		}
	}

	// Set the content type.
	// c.ContentType = (reader.ContentType())

	// Write the blob contents to the response.
	_, err = io.Copy(ctx.Writer, reader)
	if err != nil {
		ctx.AbortWithError(500, err)
		return err
	}

	if err = reader.Close(); err != nil {
		ctx.AbortWithError(500, err)
		return err
	}
	return err
}
