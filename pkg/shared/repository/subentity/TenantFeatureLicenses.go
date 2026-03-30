package subentity

// FeatureLicense holds license information for a specific feature.
type FeatureLicense struct {
	Code string `json:"code"`
}

// TenantFeatureLicenses maps feature names to their license info.
// Only features that are enabled in TenantFeatures should have an entry here.
type TenantFeatureLicenses map[string]FeatureLicense
