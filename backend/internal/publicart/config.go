package publicart

import (
	"encoding/json"
	"errors"
)

const (
	SettingsKeyConfig          = "public_art_config"
	SettingsKeySelectedArtwork = "public_art_selected_candidate" // intentionally the old key for backward compat
)

type Config struct {
	Provider               string `json:"provider"`
	Query                  string `json:"query"`
	MinImageLongEdge       int    `json:"min_image_long_edge"`
	PreferredImageLongEdge int    `json:"preferred_image_long_edge"`
	Orientation            string `json:"orientation,omitempty"`
}

type ConfigProvider interface {
	PublicArtConfig() (Config, error)
}

type SettingsGetter interface {
	Get(key string) (string, error)
}

type SettingsSetter interface {
	Set(key string, value string) error
}

type SettingsStore interface {
	SettingsGetter
	SettingsSetter
}

type SettingsConfigProvider struct {
	settings SettingsGetter
}

func NewSettingsConfigProvider(settings SettingsGetter) *SettingsConfigProvider {
	return &SettingsConfigProvider{settings: settings}
}

func (p *SettingsConfigProvider) PublicArtConfig() (Config, error) {
	if p == nil || p.settings == nil {
		return DefaultConfig(), nil
	}
	value, err := p.settings.Get(SettingsKeyConfig)
	if err != nil || value == "" {
		return DefaultConfig(), nil
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(value), &cfg); err != nil {
		return Config{}, err
	}
	return normalizeConfig(cfg), nil
}

func DefaultConfig() Config {
	return Config{
		Provider:               ProviderAIC,
		Query:                  "art",
		MinImageLongEdge:       1600,
		PreferredImageLongEdge: 2000,
	}
}

func normalizeConfig(cfg Config) Config {
	defaults := DefaultConfig()
	if cfg.Provider == "" {
		cfg.Provider = defaults.Provider
	}
	if cfg.Query == "" {
		cfg.Query = defaults.Query
	}
	if cfg.MinImageLongEdge <= 0 {
		cfg.MinImageLongEdge = defaults.MinImageLongEdge
	}
	if cfg.PreferredImageLongEdge <= 0 {
		cfg.PreferredImageLongEdge = defaults.PreferredImageLongEdge
	}
	if cfg.Orientation != "landscape" && cfg.Orientation != "portrait" {
		cfg.Orientation = ""
	}
	return cfg
}

// LoadSelectedArtwork loads the stored selected artwork with composition.
// Returns (SelectedArtwork, found, error).
// Backward compat: if stored value is a plain Candidate (no composition field),
// wraps it with DefaultComposition().
func LoadSelectedArtwork(settings SettingsGetter) (SelectedArtwork, bool, error) {
	if settings == nil {
		return SelectedArtwork{}, false, nil
	}
	value, err := settings.Get(SettingsKeySelectedArtwork)
	if err != nil || value == "" {
		return SelectedArtwork{}, false, err
	}

	// Try SelectedArtwork first
	var artwork SelectedArtwork
	if err := json.Unmarshal([]byte(value), &artwork); err == nil && artwork.Candidate.Provider != "" {
		if artwork.Composition.ScaleMode == "" {
			artwork.Composition = DefaultComposition()
		}
		if err := validateSelectedArtwork(artwork); err != nil {
			return SelectedArtwork{}, false, err
		}
		return artwork, true, nil
	}

	// Fallback: plain Candidate (backward compat)
	var candidate Candidate
	if err := json.Unmarshal([]byte(value), &candidate); err != nil {
		return SelectedArtwork{}, false, err
	}
	if err := validateSelectedCandidate(candidate); err != nil {
		return SelectedArtwork{}, false, err
	}
	return SelectedArtwork{Candidate: candidate, Composition: DefaultComposition()}, true, nil
}

// SaveSelectedArtwork stores the selected artwork with composition.
func SaveSelectedArtwork(settings SettingsSetter, artwork SelectedArtwork) error {
	if settings == nil {
		return errors.New("publicart: settings store is required")
	}
	if err := validateSelectedArtwork(artwork); err != nil {
		return err
	}
	data, err := json.Marshal(artwork)
	if err != nil {
		return err
	}
	return settings.Set(SettingsKeySelectedArtwork, string(data))
}

// ClearSelectedArtwork clears the stored selected artwork.
func ClearSelectedArtwork(settings SettingsSetter) error {
	if settings == nil {
		return errors.New("publicart: settings store is required")
	}
	return settings.Set(SettingsKeySelectedArtwork, "")
}

// validateSelectedArtwork validates a SelectedArtwork.
func validateSelectedArtwork(artwork SelectedArtwork) error {
	if err := validateSelectedCandidate(artwork.Candidate); err != nil {
		return err
	}
	if artwork.Composition.ScaleMode == "" {
		return errors.New("publicart: composition scale_mode is required")
	}
	if artwork.Composition.ScaleMode != "cover" && artwork.Composition.ScaleMode != "fit" && artwork.Composition.ScaleMode != "custom" {
		return errors.New("publicart: composition scale_mode must be cover, fit, or custom")
	}
	if artwork.Composition.Zoom < 0.1 || artwork.Composition.Zoom > 10 {
		return errors.New("publicart: composition zoom must be between 0.1 and 10")
	}
	return nil
}

func validateSelectedCandidate(candidate Candidate) error {
	if candidate.Provider == "" {
		return errors.New("publicart: selected candidate provider is required")
	}
	if candidate.ID == "" {
		return errors.New("publicart: selected candidate id is required")
	}
	if candidate.ImageURL == "" {
		return errors.New("publicart: selected candidate image_url is required")
	}
	return nil
}
