package service

import (
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

// Convention is the main entry point for all tests in package
func TestMain(m *testing.M) {
	godotenv.Overload("../../../.env", "../../../.env.local")
	os.Exit(m.Run())
}
