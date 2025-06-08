package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	standardlog "log"

	rest "ctoup.com/coreapp/internal/server/http"
	"ctoup.com/coreapp/pkg/shared/repository"
	sqlservice "ctoup.com/coreapp/pkg/shared/sql"
	_ "github.com/golang-migrate/migrate/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	_ "github.com/golang-migrate/migrate/source/file"
	// pgx/v5 with sqlc you get its implicit support for prepared statements. No additional sqlc configuration is required.

	"gopkg.in/natefinch/lumberjack.v2"
)

func main() {
	godotenv.Load("./.env")
	godotenv.Overload("./.env", "./.env.local")

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	logFolder := os.Getenv("LOG_FOLDER")
	if logFolder == "" {
		standardlog.Fatal("LOG_FOLDER required")
	}
	logFilePath := ""
	instanceName := os.Getenv("INSTANCE_NAME")
	if instanceName == "" {
		logFilePath = fmt.Sprintf("%s/main.log", logFolder)
	} else {
		logFilePath = fmt.Sprintf("%s/%s.log", logFolder, instanceName)
	}

	// Configure lumberjack for log rotation
	logFile := &lumberjack.Logger{
		Filename:   logFilePath, // Log file location
		MaxSize:    10,          // Max size in MB before rotation
		MaxBackups: 5,           // Max number of old log files to keep
		MaxAge:     30,          // Max age in days to keep a log file
		Compress:   true,        // Compress old log files
	}

	// Ensure the log file path exists or create it
	defer logFile.Close()

	// Create a multi-writer for both the console and the log file
	multiWriter := zerolog.MultiLevelWriter(logFile, os.Stdout)

	// Set Zerolog to write to the multi-writer
	log.Logger = zerolog.New(multiWriter).With().Timestamp().Logger()

	log.Print("Application started...")

	connectionString := repository.GetConnectionString()

	connector := sqlservice.ConnectorRetryDecorator{Connector: sqlservice.NewPostgresConnector(connectionString), Attempts: 1000, Delay: 5 * time.Second, IncreaseDelay: 20 * time.Millisecond, MaxDelay: 1 * time.Minute}
	log.Info().Msg("Creating Connection Pool")

	connPool, err := connector.ConnectWithRetry(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("Cannot create Pool:")
	}
	log.Info().Msg("Connection Pool created...")

	err = connPool.Ping(context.Background())
	if err != nil {
		log.Info().Msg("Cannot ping connPool. " + err.Error())
	} else {
		log.Info().Msg("Pinged!")
	}

	webstitesPort := os.Getenv("WEBSITES_PORT")
	if webstitesPort == "" {
		log.Fatal().Err(err).Msg("Please set WEBSITES_PORT")

	}
	restAddress := ":" + webstitesPort
	// Timeout for server shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	// Set up OpenTelemetry.
	/* otelShutdown, err := core.SetupOTelSDK(ctx)
	if err != nil {
		return
	}
	defer func() {
		err = errors.Join(err, otelShutdown(context.Background()))
	}() */

	go rest.RunRESTServer(ctx, connPool, restAddress, connectionString)

	// Graceful shutdown setup
	// Catch shutdown signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Gracefully shutdown Gin server
	log.Info().Msg("Shutting down server...")

	stop()

	log.Info().Msg("Server exiting")
}
