package publicart

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	Settings       SettingsGetter
	CacheDir       string
	HistoryDB      DedupHistoryDB
}

// DedupHistoryDB abstracts the GORM database operations needed for serving-history
// deduplication, so the service stays testable without a real DB.
type DedupHistoryDB interface {
	// Record adds a history entry for the given device/artwork. On DB error it logs
	// but does not fail the image fetch.
	Record(deviceID uint, source, artworkID string)
	// IsRecentlyServed returns true if the given (deviceID, source, artworkID) tuple
	// has been served within the given deduplication window (hours).
	IsRecentlyServed(deviceID uint, source, artworkID string, hours int) bool
	// Cleanup removes entries older than hours for the given device.
	Cleanup(deviceID uint, hours int)
	// CleanupExpired removes entries older than hours across all devices.
	CleanupExpired(hours int)
}

// DefaultHistoryRetentionHours is the minimum retention cap for Public Art
// serving history. Dedup uses a shorter default window (24h), but history is
// still bounded so low-traffic or dedup-disabled setups do not accumulate rows
// forever.
const DefaultHistoryRetentionHours = 24 * 30

// Service can serve artwork from the Art Institute of Chicago or other providers.
type Service struct {
	provider       Provider
	httpClient     *http.Client
	config         Config
	configProvider ConfigProvider
	settings       SettingsGetter
	cacheDir       string
	historyDB      DedupHistoryDB
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
		settings:       opts.Settings,
		cacheDir:       opts.CacheDir,
		historyDB:      opts.HistoryDB,
	}
}

func (s *Service) FetchImage(deviceID uint) (image.Image, SelectedArtwork, error) {
	return s.FetchImageWithComposition(deviceID, 0, 0)
}

func (s *Service) FetchImageWithComposition(deviceID uint, targetW, targetH int) (image.Image, SelectedArtwork, error) {
	artwork, ok, err := LoadSelectedArtwork(s.settings)
	if err != nil {
		return nil, SelectedArtwork{}, err
	}
	if ok {
		img, err := s.fetchArtworkImage(artwork.Candidate)
		if err != nil {
			return nil, SelectedArtwork{}, err
		}
		if targetW > 0 && targetH > 0 {
			comp := artwork.Composition
			if comp.ScaleMode == "" {
				comp = DefaultComposition()
			}
			img = ComposeImage(img, comp, targetW, targetH)
		}
		// Record serving for dedup even for manually-selected artwork
		s.recordServing(deviceID, artwork.Candidate.Provider, artwork.Candidate.ID)
		return img, artwork, nil
	}

	// Fall back to provider search with deduplication
	if s.provider == nil {
		return nil, SelectedArtwork{}, errors.New("publicart: provider is required")
	}
	cfg, err := s.currentConfig()
	if err != nil {
		return nil, SelectedArtwork{}, err
	}
	ranked, err := s.SearchCandidates(cfg, 10)
	if err != nil {
		return nil, SelectedArtwork{}, err
	}
	if len(ranked) == 0 {
		return nil, SelectedArtwork{}, errors.New("publicart: no image candidates found")
	}

	// Dedup: skip candidates recently served to this device
	dedupHours := DedupHours(s.settings)
	var candidate Candidate
	candidateFound := false
	for _, c := range ranked {
		if s.historyDB != nil && dedupHours > 0 && s.historyDB.IsRecentlyServed(deviceID, c.Provider, c.ID, dedupHours) {
			continue
		}
		candidate = c
		candidateFound = true
		break
	}
	if !candidateFound {
		// All candidates recently served — allow repeat rather than error
		candidate = ranked[0]
	}

	img, err := s.fetchArtworkImage(candidate)
	if err != nil {
		return nil, SelectedArtwork{}, err
	}
	// Apply default cover composition
	if targetW > 0 && targetH > 0 {
		img = ComposeImage(img, DefaultComposition(), targetW, targetH)
	}
	s.recordServing(deviceID, candidate.Provider, candidate.ID)
	return img, SelectedArtwork{Candidate: candidate, Composition: DefaultComposition()}, nil
}

func (s *Service) SearchCandidates(cfg Config, limit int) ([]Candidate, error) {
	if s.provider == nil {
		return nil, errors.New("publicart: provider is required")
	}
	cfg = normalizeConfig(cfg)
	searchLimit := limit
	if searchLimit < 10 {
		searchLimit = 10
	}
	if cfg.Orientation == "landscape" || cfg.Orientation == "portrait" {
		// Museum search APIs do not necessarily rank by orientation. Over-fetch so
		// the orientation filter has enough candidates to choose from instead of
		// falling back to the wrong aspect ratio.
		if searchLimit < limit*5 {
			searchLimit = limit * 5
		}
		if searchLimit < 50 {
			searchLimit = 50
		}
	}
	candidates, err := s.provider.Search(cfg.Query, SearchOptions{Limit: searchLimit})
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 && cfg.Query != DefaultConfig().Query {
		candidates, err = s.provider.Search(DefaultConfig().Query, SearchOptions{Limit: searchLimit})
		if err != nil {
			return nil, err
		}
	}
	ranked := RankCandidates(candidates, cfg)
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked, nil
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
	data, err := s.downloadImageBytes(imageURL)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("publicart: decode image: %w", err)
	}
	return img, nil
}

func (s *Service) downloadImageBytes(imageURL string) ([]byte, error) {
	if imageURL == "" {
		return nil, errors.New("publicart: image URL is required")
	}
	if data, ok, err := DecodeDataImageURL(imageURL); ok {
		return data, err
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
	return io.ReadAll(io.LimitReader(resp.Body, maxImageDownloadBytes))
}

func (s *Service) fetchArtworkImage(candidate Candidate) (image.Image, error) {
	if s.cacheDir != "" {
		if data, ok := s.readCachedImage(candidate); ok {
			if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
				return img, nil
			}
		}
	}

	data, err := s.downloadImageBytes(candidate.ImageURL)
	if err != nil && candidate.ThumbnailURL != "" {
		data, err = s.downloadImageBytes(candidate.ThumbnailURL)
	}
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("publicart: decode image: %w", err)
	}
	if s.cacheDir != "" {
		_ = s.writeCachedImage(candidate, data)
	}
	return img, nil
}

func (s *Service) readCachedImage(candidate Candidate) ([]byte, bool) {
	path := s.cachePath(candidate)
	if path == "" {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}

func (s *Service) writeCachedImage(candidate Candidate, data []byte) error {
	path := s.cachePath(candidate)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.cacheDir, ".public-art-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	s.cleanupOtherCacheFiles(filepath.Base(path))
	return nil
}

func (s *Service) cleanupOtherCacheFiles(keep string) {
	entries, err := os.ReadDir(s.cacheDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == keep || !strings.HasSuffix(name, ".img") {
			continue
		}
		_ = os.Remove(filepath.Join(s.cacheDir, name))
	}
}

func (s *Service) cachePath(candidate Candidate) string {
	if s.cacheDir == "" || candidate.ImageURL == "" {
		return ""
	}
	h := sha256.Sum256([]byte(candidate.Provider + "\x00" + candidate.ID + "\x00" + candidate.ImageURL))
	return filepath.Join(s.cacheDir, hex.EncodeToString(h[:])+".img")
}

// recordServing writes a serving history entry and cleans up old entries.
// Errors are logged but do not propagate — dedup is best-effort.
func (s *Service) recordServing(deviceID uint, source, artworkID string) {
	if s.historyDB == nil || deviceID == 0 {
		return
	}
	s.historyDB.Record(deviceID, source, artworkID)
	// Also clean up entries older than dedup window for this device.
	hours := DedupHours(s.settings)
	if hours > 0 {
		s.historyDB.Cleanup(deviceID, hours)
	}

	// Independently cap global history retention, even when dedup is disabled
	// (hours == 0) or a device stops fetching images. If the user sets a dedup
	// window longer than the default retention, retain enough data to honor it.
	retentionHours := DefaultHistoryRetentionHours
	if hours > retentionHours {
		retentionHours = hours
	}
	s.historyDB.CleanupExpired(retentionHours)
}
