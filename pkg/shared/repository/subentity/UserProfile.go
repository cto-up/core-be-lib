package subentity

type UserProfile struct {
	Name                 string   `json:"name"`
	Title                string   `json:"title"`
	About                string   `json:"about"`
	PictureURL           string   `json:"pictureURL"`
	BackgroundPictureURL string   `json:"backgroundPictureURL"`
	SocialMedias         []string `json:"socialMedias"`
	Interests            []string `json:"interests"`
	Skills               []string `json:"skills"`
}
