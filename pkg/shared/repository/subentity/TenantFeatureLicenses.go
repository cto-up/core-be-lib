package subentity

import "time"

// FeatureLicense holds license information for a specific feature.
type FeatureLicense struct {
	Code string `json:"code"`
	// EndDate is the optional expiry date of the feature license. When set and
	// reached, the feature is disabled. Nil means the license never expires.
	EndDate *time.Time `json:"end_date,omitempty"`
}

// TenantFeatureLicenses maps feature names to their license info.
// Only features that are enabled in TenantFeatures should have an entry here.
type TenantFeatureLicenses map[string]FeatureLicense
