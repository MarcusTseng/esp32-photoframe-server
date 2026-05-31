package publicart

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAICProviderSearchBuildsCandidatesWithIIIFImageURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/artworks/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "monet" {
			t.Fatalf("q = %q, want monet", got)
		}
		if got := r.URL.Query().Get("fields"); got == "" {
			t.Fatalf("fields query should be set")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"config": {"iiif_url": "https://www.artic.edu/iiif/2"},
			"data": [
				{"id": 1, "title": "Water Lilies", "artist_title": "Claude Monet", "date_display": "1906", "image_id": "abc123", "thumbnail": {"width": 3000, "height": 2000}},
				{"id": 2, "title": "No Image", "artist_title": "Unknown", "image_id": null}
			]
		}`))
	}))
	defer server.Close()

	provider := NewAICProvider(server.URL, server.Client())
	candidates, err := provider.Search("monet", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(candidates))
	}

	got := candidates[0]
	if got.Provider != ProviderAIC {
		t.Fatalf("Provider = %q, want %q", got.Provider, ProviderAIC)
	}
	if got.ID != "aic:1" {
		t.Fatalf("ID = %q, want aic:1", got.ID)
	}
	if got.Title != "Water Lilies" || got.Artist != "Claude Monet" || got.Date != "1906" {
		t.Fatalf("unexpected metadata: %#v", got)
	}
	wantURL := "https://www.artic.edu/iiif/2/abc123/full/2000,/0/default.jpg"
	if got.ImageURL != wantURL {
		t.Fatalf("ImageURL = %q, want %q", got.ImageURL, wantURL)
	}
	if got.Width != 3000 || got.Height != 2000 {
		t.Fatalf("dimensions = %dx%d, want 3000x2000", got.Width, got.Height)
	}
}

func TestAICProviderSearchRejectsBlankQuery(t *testing.T) {
	provider := NewAICProvider("https://example.invalid", nil)
	_, err := provider.Search("   ", SearchOptions{})
	if err == nil {
		t.Fatal("Search returned nil error for blank query")
	}
}
