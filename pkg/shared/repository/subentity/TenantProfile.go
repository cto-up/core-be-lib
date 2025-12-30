package subentity

import "ctoup.com/coreapp/api/openapi/core"

type TenantProfile struct {
	DisplayName string           `json:"displayName"`
	Values      string           `json:"values"`
	LightColors core.ColorSchema `json:"lightColors"`
	DarkColors  core.ColorSchema `json:"darkColors"`
}
