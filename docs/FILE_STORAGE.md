# File Management

File storage should work across file system, and google, aws, and azure cloud providers.

It was tested with google cloud storage and file system.

## ETag Caching

The file management service supports ETag caching. The ETag is generated from the file content hash.

## File Management Service

The file management service is responsible for storing and retrieving files. It is implemented in `pkg/shared/fileservice/file_service.go`.

## File Management Service Usage

The file management service is used in the following places:

- User profile pictures
- Tenant pictures

## File Storage Locations

### User Profile Pictures

path `/tenants/{tenantId}/core/users/{userId}/profile-picture.jpg`

### Tenant Pictures

path `/tenants/{tenantId}/core/pictures/{pictureType}.webp`

## File System Configuration

```
FILE_STORAGE_PROVIDER=file
FILE_FOLDER_URL=/path/to/folder
```

## Google Cloud Storage Configuration

```
FILE_STORAGE_PROVIDER=gcs
GCS_BUCKET_NAME=your-bucket-name
GOOGLE_APPLICATION_CREDENTIALS=/path/to/credentials.json
```

# Cloud Provider Credentials Tutorial

This guide provides a step-by-step tutorial on how to obtain the required environment variables for Google Cloud, Amazon Web Services, and Microsoft Azure to connect your application to their respective storage services.

## Google Cloud Storage (GCS)

To authenticate with Google Cloud Storage, you'll use a service account key file. The GOOGLE_APPLICATION_CREDENTIALS environment variable should point to the path of this JSON file.

Steps:

1. Create a Service Account:

Navigate to the Google Cloud Console.

In the navigation menu, go to IAM & Admin > Service Accounts.

Click + CREATE SERVICE ACCOUNT.

Give your service account a name (e.g., go-storage-app) and a description. Click DONE.

2. Grant Permissions to the Service Account:

Go to IAM & Admin > IAM and click GRANT ACCESS.

In the "New principals" field, search for the service account you just created.

Assign the role Storage Object Admin to allow it to read and write objects in your bucket. Click SAVE.

3. Generate a JSON Key File:

Go back to IAM & Admin > Service Accounts.

Click on the service account you created.

Go to the KEYS tab and click ADD KEY > Create new key.

Select JSON as the key type and click CREATE.

Your browser will download a JSON file. Save this file in a secure location on your machine. Do not commit this file to your source code repository.

Set the Environment Variable:

Open your terminal and set the GOOGLE_APPLICATION_CREDENTIALS variable to the path of the downloaded JSON file.

export GOOGLE_APPLICATION_CREDENTIALS=/path/to/your-key-file.json

Now, you can also set the bucket name for your application.

export GCS_BUCKET_NAME=your-gcs-bucket-name

## Amazon S3 (AWS)

For S3, you'll need an access key ID, a secret access key, and a region. These are typically associated with an IAM user.

Steps:

Create an IAM User:

Navigate to the AWS Management Console.

Go to IAM > Users and click Create user.

Provide a user name (e.g., go-storage-user).

Select Next, then choose Attach policies directly to set permissions.

Search for and select the AmazonS3FullAccess policy. Click Next and then Create user.

Generate Access Keys:

Once the user is created, click on the user's name to view their details.

Go to the Security credentials tab and click Create access key.

Select Command Line Interface (CLI) as the use case, acknowledge the warning, and click Next.

Click Create access key.

You will be shown the Access key ID and Secret access key. Copy these immediately, as you will not be able to retrieve the secret key again.

Set the Environment Variables:

Open your terminal and set the following variables with your new credentials and the bucket's region. You can find the region of your bucket in the S3 console.

export AWS_ACCESS_KEY_ID=your-access-key-id
export AWS_SECRET_ACCESS_KEY=your-secret-access-key
export AWS_REGION=us-east-1 # Example region
export S3_BUCKET_NAME=your-s3-bucket-name

## Azure Blob Storage

Azure Blob Storage uses a storage account name and an account key for authentication.

Steps:

Create a Storage Account:

Go to the Azure portal.

In the search bar, search for "Storage accounts" and click Create.

Select your subscription and resource group.

Provide a unique name for your storage account (e.g., gostorageapp).

Fill out the rest of the form and click Review + Create, then Create.

Find the Account Key:

Once the storage account is created, navigate to its page.

In the left navigation menu, under "Security + networking", select Access keys.

Click Show keys. You will see key1 and key2. You can use either one.

Copy the Storage account name and one of the Key values.

Set the Environment Variables:

Open your terminal and set the environment variables.

export AZURE_STORAGE_ACCOUNT=your-storage-account-name
export AZURE_STORAGE_KEY=your-storage-key
export AZURE_STORAGE_CONT
