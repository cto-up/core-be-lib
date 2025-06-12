package subentity

type TenantProfile struct {
	StoreRAGDocument bool   `json:"storeRAGDocument"`
	DisplayName      string `json:"displayName"`
	Values           string `json:"values"`
	LightColors      Colors `json:"lightColors"`
	DarkColors       Colors `json:"darkColors"`
}

type Colors struct {
	Background string `json:"background"`
	Primary    string `json:"primary"`
	Secondary  string `json:"secondary"`
	Tertiary   string `json:"tertiary"`
	Accent     string `json:"accent"`
	Positive   string `json:"positive"`
	Negative   string `json:"negative"`
	Info       string `json:"info"`
	Warning    string `json:"warning"`
	Text       string `json:"text"`
}
