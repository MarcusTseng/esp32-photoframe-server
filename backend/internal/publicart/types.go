package publicart

const (
	ProviderAIC = "aic"
)

type Candidate struct {
	Provider  string `json:"provider"`
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist,omitempty"`
	Date      string `json:"date,omitempty"`
	ImageURL  string `json:"image_url"`
	SourceURL string `json:"source_url,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

type SearchOptions struct {
	Limit int
}
