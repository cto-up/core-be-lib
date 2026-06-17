package subentity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEffectiveFeatures(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	past := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)

	features := TenantFeatures{
		"skeellscoach":   true,
		"skeellstrainer": true,
		"skeellsfriend":  true,
	}
	licenses := TenantFeatureLicenses{
		"skeellscoach":   {Code: "COACH", EndDate: &past},   // expired -> disabled
		"skeellstrainer": {Code: "TRAIN", EndDate: &future}, // not yet expired -> unchanged
		"skeellsfriend":  {Code: "FRIEND"},                  // no end date -> never expires
	}

	got := EffectiveFeatures(features, licenses, now)

	assert.False(t, got["skeellscoach"], "feature with a passed end_date must be disabled")
	assert.True(t, got["skeellstrainer"], "feature with a future end_date must stay enabled")
	assert.True(t, got["skeellsfriend"], "feature without an end_date must stay enabled")

	// the input map must not be mutated
	assert.True(t, features["skeellscoach"], "EffectiveFeatures must not mutate the input")
}

func TestEffectiveFeaturesIgnoresLicenseForDisabledFeature(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	past := now.Add(-24 * time.Hour)

	features := TenantFeatures{"skeellscoach": false}
	licenses := TenantFeatureLicenses{"skeellscoach": {Code: "COACH", EndDate: &past}}

	got := EffectiveFeatures(features, licenses, now)
	assert.False(t, got["skeellscoach"])
}
