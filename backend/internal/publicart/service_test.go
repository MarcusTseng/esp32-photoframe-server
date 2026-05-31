package publicart

import (
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
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
	if selected.ID != "large" {
		t.Fatalf("selected ID = %q, want large", selected.ID)
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
