package testutils

import (
	"os"
	"testing"

	"ctoup.com/coreapp/pkg/core/db"
	"github.com/rs/zerolog/log"
)

var testStore *db.Store

// Convention is the main entry point for all tests in package
func TestMain(m *testing.M) {
	store, cleanup, err := NewMigratedStore()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to set up the test store")
	}
	testStore = store

	code := m.Run()
	cleanup()
	os.Exit(code)
}
