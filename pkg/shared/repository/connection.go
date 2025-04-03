package repository

import (
	"fmt"

	"github.com/rs/zerolog/log"

	"os"
)

func GetConnectionString() string {

	username, password, databaseUrl := getConnectionInfo()

	return fmt.Sprintf("postgres://%s:%s@%s", username, password, databaseUrl)
}

func getConnectionInfo() (string, string, string) {
	username := os.Getenv("DATABASE_USERNAME")
	if username == "" {
		log.Fatal().Msg("DATABASE_USERNAME required")
	}
	password := os.Getenv("DATABASE_PASSWORD")
	if password == "" {
		log.Fatal().Msg("DATABASE_PASSWORD required")
	}
	databaseUrl := os.Getenv("DATABASE_URL")
	if databaseUrl == "" {
		log.Fatal().Msg("DATABASE_URL required")
	}
	return username, password, databaseUrl
}
