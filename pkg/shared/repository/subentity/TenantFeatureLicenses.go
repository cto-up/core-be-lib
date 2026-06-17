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

// EffectiveFeatures returns a copy of features with any feature whose license
// EndDate has passed (relative to now) forced to false. It is value-agnostic:
// it never references specific feature names. Callers use it to guard access
// in real time without waiting for the background job that persists the change.
func EffectiveFeatures(features TenantFeatures, licenses TenantFeatureLicenses, now time.Time) TenantFeatures {
	effective := make(TenantFeatures, len(features))
	for name, enabled := range features {
		effective[name] = enabled
	}
	for name, license := range licenses {
		if license.EndDate != nil && license.EndDate.Before(now) {
			effective[name] = false
		}
	}
	return effective
}
