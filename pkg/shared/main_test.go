package service

import (
	"os"
	"testing"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// Convention is the main entry point for all tests in package
func TestMain(m *testing.M) {
	godotenv.Overload("../../../.env", "../../../.env.local")
	os.Exit(m.Run())
}
