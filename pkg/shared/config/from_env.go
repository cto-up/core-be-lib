package config

import "os"

// FromEnv reads the deployment's configuration from the process environment.
//
// This is the ONE place in the library that reads os.Getenv, and it is opt-in:
// a host adopts it with config.Set(config.FromEnv()), or supplies its own
// Config and this file never runs. That is the difference between a library
// that offers an environment strategy and one that has an environment
// dependency.
//
// Every variable keeps the name it has always had, so no deployment has to
// change anything to adopt this.
//
// Every one of these must reach BOTH replicas. The backend compose has no
// env_file, so a value set only under `app` silently differs on `app_replica`.
func FromEnv() Config {
	return Config{
		Site: Site{
			Domain: os.Getenv("DOMAIN"),
		},
		Email: Email{
			SystemFrom: os.Getenv("SYSTEM_EMAIL"),
		},
		Database: Database{
			URL:      os.Getenv("DATABASE_URL"),
			Username: os.Getenv("DATABASE_USERNAME"),
			Password: os.Getenv("DATABASE_PASSWORD"),
		},
		Kratos: Kratos{
			AdminURL:  os.Getenv("KRATOS_ADMIN_URL"),
			PublicURL: os.Getenv("KRATOS_PUBLIC_URL"),
		},
		Seed: Seed{
			UserEmail:    os.Getenv("SEED_USER_EMAIL"),
			UserPassword: os.Getenv("SEED_USER_PASSWORD"),
		},
		Sentry: Sentry{
			DSN:              os.Getenv("SENTRY_DSN"),
			Environment:      os.Getenv("SENTRY_ENVIRONMENT"),
			Release:          os.Getenv("SENTRY_RELEASE"),
			TracesSampleRate: os.Getenv("SENTRY_TRACES_SAMPLE_RATE"),
		},
		Turn: Turn{
			PublicIP: os.Getenv("BACKEND_HOST"),
			Port:     os.Getenv("TURN_SERVER_PORT"),
		},
		Storage: Storage{
			Provider:     os.Getenv("FILE_STORAGE_PROVIDER"),
			Bucket:       storageBucketFromEnv(),
			LocalPath:    os.Getenv("FILE_FOLDER_URL"),
			AWSRegion:    os.Getenv("AWS_REGION"),
			AzureAccount: os.Getenv("AZURE_STORAGE_ACCOUNT"),
			AzureKey:     os.Getenv("AZURE_STORAGE_KEY"),
			// Preserves today's behaviour. Turning it off is a deployment's
			// choice, not a silent change — see Storage.BootstrapBucket.
			BootstrapBucket: true,
		},
		SMTP: SMTP{
			Host:     os.Getenv("SMTP_HOST"),
			Port:     os.Getenv("SMTP_PORT"),
			Username: os.Getenv("SMTP_USERNAME"),
			Password: os.Getenv("SMTP_PASSWORD"),
		},
	}
}

// storageBucketFromEnv picks the bucket variable matching the selected provider.
// Three names for one concept is how the environment happens to spell it; the
// library sees one field.
func storageBucketFromEnv() string {
	switch os.Getenv("FILE_STORAGE_PROVIDER") {
	case "gcs":
		return os.Getenv("GCS_BUCKET_NAME")
	case "s3":
		return os.Getenv("S3_BUCKET_NAME")
	case "azure":
		return os.Getenv("AZURE_STORAGE_CONTAINER_NAME")
	default:
		return ""
	}
}
