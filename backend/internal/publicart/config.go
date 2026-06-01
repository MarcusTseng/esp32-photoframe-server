package publicart

import (
	"encoding/json"
	"errors"
)

const (
	SettingsKeyConfig            = "public_art_config"
	SettingsKeySelectedCandidate = "public_art_selected_candidate"
)

type Config struct {
	Provider               string `json:"provider"`
	Query                  string `json:"query"`
	MinImageLongEdge       int    `json:"min_image_long_edge"`
	PreferredImageLongEdge int    `json:"preferred_image_long_edge"`
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
	return cfg
}

func LoadSelectedCandidate(settings SettingsGetter) (Candidate, bool, error) {
	if settings == nil {
		return Candidate{}, false, nil
	}
	value, err := settings.Get(SettingsKeySelectedCandidate)
	if err != nil || value == "" {
		return Candidate{}, false, err
	}
	var candidate Candidate
	if err := json.Unmarshal([]byte(value), &candidate); err != nil {
		return Candidate{}, false, err
	}
	if err := validateSelectedCandidate(candidate); err != nil {
		return Candidate{}, false, err
	}
	return candidate, true, nil
}

func SaveSelectedCandidate(settings SettingsSetter, candidate Candidate) error {
	if settings == nil {
		return errors.New("publicart: settings store is required")
	}
	if err := validateSelectedCandidate(candidate); err != nil {
		return err
	}
	data, err := json.Marshal(candidate)
	if err != nil {
		return err
	}
	return settings.Set(SettingsKeySelectedCandidate, string(data))
}

func ClearSelectedCandidate(settings SettingsSetter) error {
	if settings == nil {
		return errors.New("publicart: settings store is required")
	}
	return settings.Set(SettingsKeySelectedCandidate, "")
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
