package publicart

import (
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

type fakeProvider struct {
	candidates []Candidate
	err        error
	queries    []string
}

func (p *fakeProvider) Search(query string, opts SearchOptions) ([]Candidate, error) {
	p.queries = append(p.queries, query)
	return p.candidates, p.err
}

func TestServiceFetchImageSearchesRanksAndDecodesImage(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		img := image.NewRGBA(image.Rect(0, 0, 2, 1))
		img.Set(0, 0, color.RGBA{R: 255, A: 255})
		img.Set(1, 0, color.RGBA{B: 255, A: 255})
		w.Header().Set("Content-Type", "image/png")
		if err := png.Encode(w, img); err != nil {
			t.Fatalf("encode png: %v", err)
		}
	}))
	defer imageServer.Close()

	provider := &fakeProvider{candidates: []Candidate{
		{ID: "small", Title: "Small", ImageURL: imageServer.URL, Width: 800, Height: 600},
		{ID: "large", Title: "Large", ImageURL: imageServer.URL, Width: 3000, Height: 2000},
	}}
	svc := NewService(ServiceOptions{
		Provider:   provider,
		HTTPClient: imageServer.Client(),
		Config:     DefaultConfig(),
	})

	img, selected, err := svc.FetchImage()
	if err != nil {
		t.Fatalf("FetchImage returned error: %v", err)
	}
	if selected.Candidate.ID != "large" {
		t.Fatalf("selected ID = %q, want large", selected.Candidate.ID)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 2 || bounds.Dy() != 1 {
		t.Fatalf("decoded image size = %dx%d, want 2x1", bounds.Dx(), bounds.Dy())
	}
}

func TestServiceFetchImageReturnsErrorWhenNoCandidates(t *testing.T) {
	provider := &fakeProvider{}
	svc := NewService(ServiceOptions{
		Provider: provider,
		Config:   DefaultConfig(),
	})
	_, _, err := svc.FetchImage()
	if err == nil {
		t.Fatal("FetchImage returned nil error for no candidates")
	}
}

func TestServiceFetchImageUsesLatestConfigFromProvider(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		w.Header().Set("Content-Type", "image/png")
		if err := png.Encode(w, img); err != nil {
			t.Fatalf("encode png: %v", err)
		}
	}))
	defer imageServer.Close()

	provider := &fakeProvider{candidates: []Candidate{{ID: "large", Title: "Large", ImageURL: imageServer.URL, Width: 3000, Height: 2000}}}
	cfgProvider := &fakeConfigProvider{configs: []Config{
		{Provider: ProviderAIC, Query: "monet", MinImageLongEdge: 1600, PreferredImageLongEdge: 2000},
		{Provider: ProviderAIC, Query: "hokusai", MinImageLongEdge: 1600, PreferredImageLongEdge: 2000},
	}}
	svc := NewService(ServiceOptions{
		Provider:       provider,
		HTTPClient:     imageServer.Client(),
		ConfigProvider: cfgProvider,
	})

	if _, _, err := svc.FetchImage(); err != nil {
		t.Fatalf("first FetchImage returned error: %v", err)
	}
	if _, _, err := svc.FetchImage(); err != nil {
		t.Fatalf("second FetchImage returned error: %v", err)
	}
	if got, want := provider.queries, []string{"monet", "hokusai"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("provider queries = %#v, want %#v", got, want)
	}
}

func TestServiceSearchCandidatesRanksAndLimitsResults(t *testing.T) {
	provider := &fakeProvider{candidates: []Candidate{
		{ID: "small", Title: "Small", Width: 800, Height: 600},
		{ID: "large", Title: "Large", Width: 3000, Height: 2000},
		{ID: "minimum", Title: "Minimum", Width: 1600, Height: 900},
	}}
	svc := NewService(ServiceOptions{Provider: provider, Config: DefaultConfig()})

	candidates, err := svc.SearchCandidates(Config{Provider: ProviderAIC, Query: "monet", MinImageLongEdge: 1600, PreferredImageLongEdge: 2000}, 2)
	if err != nil {
		t.Fatalf("SearchCandidates returned error: %v", err)
	}
	if got, want := provider.queries, []string{"monet"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("provider queries = %#v, want %#v", got, want)
	}
	if got, want := len(candidates), 2; got != want {
		t.Fatalf("len(candidates) = %d, want %d", got, want)
	}
	if candidates[0].ID != "large" || candidates[1].ID != "minimum" {
		t.Fatalf("candidate order = %#v, want large then minimum", candidates)
	}
}

func TestServiceFetchImageSelectedCandidateBypassesProviderAndUsesCache(t *testing.T) {
	requests := 0
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		writeTinyPNG(t, w)
	}))
	defer imageServer.Close()

	store := &fakeSettingsStore{}
	selected := Candidate{Provider: ProviderAIC, ID: "aic:selected", Title: "Selected", ImageURL: imageServer.URL}
	if err := SaveSelectedArtwork(store, SelectedArtwork{Candidate: selected, Composition: DefaultComposition()}); err != nil {
		t.Fatalf("SaveSelectedArtwork returned error: %v", err)
	}
	provider := &fakeProvider{candidates: []Candidate{{ID: "provider", ImageURL: imageServer.URL, Width: 3000, Height: 2000}}}
	svc := NewService(ServiceOptions{
		Provider:   provider,
		HTTPClient: imageServer.Client(),
		Settings:   store,
		CacheDir:   t.TempDir(),
	})

	_, got, err := svc.FetchImage()
	if err != nil {
		t.Fatalf("first FetchImage returned error: %v", err)
	}
	if got.Candidate.ID != selected.ID {
		t.Fatalf("selected ID = %q, want %q", got.Candidate.ID, selected.ID)
	}
	if len(provider.queries) != 0 {
		t.Fatalf("provider queries = %#v, want none", provider.queries)
	}
	if requests != 1 {
		t.Fatalf("requests after first fetch = %d, want 1", requests)
	}

	if _, _, err := svc.FetchImage(); err != nil {
		t.Fatalf("second FetchImage returned error: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests after cache hit = %d, want 1", requests)
	}
}

func TestServiceFetchImageBadCacheFallsBackToDownload(t *testing.T) {
	requests := 0
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		writeTinyPNG(t, w)
	}))
	defer imageServer.Close()

	store := &fakeSettingsStore{}
	selected := Candidate{Provider: ProviderAIC, ID: "aic:selected", Title: "Selected", ImageURL: imageServer.URL}
	if err := SaveSelectedArtwork(store, SelectedArtwork{Candidate: selected, Composition: DefaultComposition()}); err != nil {
		t.Fatalf("SaveSelectedArtwork returned error: %v", err)
	}
	cacheDir := t.TempDir()
	svc := NewService(ServiceOptions{HTTPClient: imageServer.Client(), Settings: store, CacheDir: cacheDir})
	if err := os.WriteFile(svc.cachePath(selected), []byte("not an image"), 0o644); err != nil {
		t.Fatalf("write bad cache: %v", err)
	}

	if _, _, err := svc.FetchImage(); err != nil {
		t.Fatalf("FetchImage returned error: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1 fallback download", requests)
	}
}

func writeTinyPNG(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{B: 255, A: 255})
	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
}

type fakeConfigProvider struct {
	configs []Config
	calls   int
}

func (p *fakeConfigProvider) PublicArtConfig() (Config, error) {
	idx := p.calls
	if idx >= len(p.configs) {
		idx = len(p.configs) - 1
	}
	p.calls++
	return p.configs[idx], nil
}
