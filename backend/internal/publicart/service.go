package publicart

import (
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

const maxImageDownloadBytes = 20 << 20

type Provider interface {
	Search(query string, opts SearchOptions) ([]Candidate, error)
}

type ServiceOptions struct {
	Provider       Provider
	HTTPClient     *http.Client
	Config         Config
	ConfigProvider ConfigProvider
}

type Service struct {
	provider       Provider
	httpClient     *http.Client
	config         Config
	configProvider ConfigProvider
}

func NewService(opts ServiceOptions) *Service {
	cfg := opts.Config
	if cfg.Provider == "" && cfg.Query == "" && cfg.MinImageLongEdge == 0 && cfg.PreferredImageLongEdge == 0 {
		cfg = DefaultConfig()
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Service{
		provider:       opts.Provider,
		httpClient:     client,
		config:         normalizeConfig(cfg),
		configProvider: opts.ConfigProvider,
	}
}

func (s *Service) FetchImage() (image.Image, Candidate, error) {
	if s.provider == nil {
		return nil, Candidate{}, errors.New("publicart: provider is required")
	}
	cfg, err := s.currentConfig()
	if err != nil {
		return nil, Candidate{}, err
	}
	candidates, err := s.provider.Search(cfg.Query, SearchOptions{Limit: 10})
	if err != nil {
		return nil, Candidate{}, err
	}
	ranked := RankCandidates(candidates, cfg)
	if len(ranked) == 0 {
		return nil, Candidate{}, errors.New("publicart: no image candidates found")
	}
	selected := ranked[0]
	img, err := s.downloadImage(selected.ImageURL)
	if err != nil {
		return nil, Candidate{}, err
	}
	return img, selected, nil
}

func (s *Service) currentConfig() (Config, error) {
	if s.configProvider == nil {
		return normalizeConfig(s.config), nil
	}
	cfg, err := s.configProvider.PublicArtConfig()
	if err != nil {
		return Config{}, err
	}
	return normalizeConfig(cfg), nil
}

func (s *Service) downloadImage(imageURL string) (image.Image, error) {
	if imageURL == "" {
		return nil, errors.New("publicart: image URL is required")
	}
	req, err := http.NewRequest(http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	setBrowserLikeHeaders(req)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("publicart: download image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("publicart: image status %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, maxImageDownloadBytes)
	img, _, err := image.Decode(limited)
	if err != nil {
		return nil, fmt.Errorf("publicart: decode image: %w", err)
	}
	return img, nil
}
