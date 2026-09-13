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
		SMTP: SMTP{
			Host:     os.Getenv("SMTP_HOST"),
			Port:     os.Getenv("SMTP_PORT"),
			Username: os.Getenv("SMTP_USERNAME"),
			Password: os.Getenv("SMTP_PASSWORD"),
		},
	}
}
