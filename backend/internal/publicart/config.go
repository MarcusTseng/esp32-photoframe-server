package publicart

import "encoding/json"

const SettingsKeyConfig = "public_art_config"

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
