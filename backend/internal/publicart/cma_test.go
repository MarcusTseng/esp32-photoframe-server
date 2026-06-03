package publicart

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCMAProviderSearchReturnsUsableImageURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("has_image"); got != "1" {
			t.Fatalf("has_image = %q, want 1", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [{
				"id": 135382,
				"title": "The Red Kerchief",
				"creation_date": "c. 1868–73",
				"url": "https://clevelandart.org/art/1958.39",
				"creators": [{"description": "Claude Monet (French, 1840–1926)"}],
				"images": {
					"web": {"url": "https://cdn.example/web.jpg", "width": "778", "height": "893"},
					"print": {"url": "https://cdn.example/print.jpg", "width": "1536", "height": "1931"}
				}
			}]
		}`))
	}))
	defer server.Close()

	provider := NewCMAProvider(server.URL, server.Client())
	candidates, err := provider.Search("monet", SearchOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(candidates))
	}
	got := candidates[0]
	if got.Provider != ProviderCMA || got.ID != "cma:135382" {
		t.Fatalf("candidate identity = %s %s, want cma cma:135382", got.Provider, got.ID)
	}
	if got.ImageURL != "https://cdn.example/print.jpg" {
		t.Fatalf("ImageURL = %q, want print URL", got.ImageURL)
	}
	if got.ThumbnailURL != "https://cdn.example/web.jpg" {
		t.Fatalf("ThumbnailURL = %q, want web URL", got.ThumbnailURL)
	}
	if got.Width != 1536 || got.Height != 1931 {
		t.Fatalf("size = %dx%d, want 1536x1931", got.Width, got.Height)
	}
}

func TestCMAProviderSearchSkipsRecordsWithoutImageURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":1,"title":"No Image","images":{}}]}`))
	}))
	defer server.Close()

	provider := NewCMAProvider(server.URL, server.Client())
	candidates, err := provider.Search("missing", SearchOptions{})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("len(candidates) = %d, want 0", len(candidates))
	}
}
