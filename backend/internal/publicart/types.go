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

// Composition describes how an artwork is fitted to the target display.
type Composition struct {
	ScaleMode       string  `json:"scale_mode"`        // "cover" | "fit" | "custom"
	Zoom            float64 `json:"zoom"`              // 1.0 = 100%, 2.0 = 200%
	PanX            float64 `json:"pan_x"`             // -0.5 to 0.5 (fraction of image center shift)
	PanY            float64 `json:"pan_y"`             // -0.5 to 0.5
	BackgroundColor string  `json:"background_color"` // e.g. "white", "black", "#1a1a1a"
}

// SelectedArtwork pairs a candidate artwork with user-defined composition.
type SelectedArtwork struct {
	Candidate   Candidate   `json:"candidate"`
	Composition Composition `json:"composition"`
}

// DefaultComposition returns sensible defaults for new selections.
func DefaultComposition() Composition {
	return Composition{
		ScaleMode:       "cover",
		Zoom:            1.0,
		PanX:            0,
		PanY:            0,
		BackgroundColor: "white",
	}
}
