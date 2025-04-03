package subentity

type TenantFeatures struct {
	Projects       bool `json:"projects"`
	Recruitment    bool `json:"recruitment"`
	SeriousGames   bool `json:"seriousGames"`
	RAGDocuments   bool `json:"RAGDocuments"`
	DemoComponents bool `json:"demoComponents"`
	DemoLearning   bool `json:"demoLearning"`
	Automation     bool `json:"automation"`
	Skeellscoach   bool `json:"skeellscoach"`
}
