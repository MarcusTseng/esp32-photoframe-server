package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/publicart"
	"github.com/labstack/echo/v4"
)

type fakePublicArtSearchService struct {
	cfg   publicart.Config
	limit int
}

func (s *fakePublicArtSearchService) SearchCandidates(cfg publicart.Config, limit int) ([]publicart.Candidate, error) {
	s.cfg = cfg
	s.limit = limit
	return []publicart.Candidate{{ID: "aic:1", Title: "Water Lilies", Width: 3000, Height: 2000}}, nil
}

type fakePublicArtSettingsStore struct {
	values map[string]string
}

func (s *fakePublicArtSettingsStore) Get(key string) (string, error) {
	if s.values == nil {
		return "", nil
	}
	return s.values[key], nil
}

func (s *fakePublicArtSettingsStore) Set(key string, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func TestPublicArtSearchReturnsRankedCandidates(t *testing.T) {
	e := echo.New()
	svc := &fakePublicArtSearchService{}
	h := NewPublicArtHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/public-art/search", strings.NewReader(`{"query":"monet","limit":5,"orientation":"portrait"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	if err := h.Search(e.NewContext(req, rec)); err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if svc.cfg.Query != "monet" {
		t.Fatalf("query = %q, want monet", svc.cfg.Query)
	}
	if svc.cfg.Orientation != "portrait" {
		t.Fatalf("orientation = %q, want portrait", svc.cfg.Orientation)
	}
	if svc.limit != 5 {
		t.Fatalf("limit = %d, want 5", svc.limit)
	}
	var candidates []publicart.Candidate
	if err := json.Unmarshal(rec.Body.Bytes(), &candidates); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != "aic:1" {
		t.Fatalf("candidates = %#v, want aic:1", candidates)
	}
}

func TestPublicArtSelectStoresArtworkWithComposition(t *testing.T) {
	e := echo.New()
	settings := &fakePublicArtSettingsStore{}
	h := NewPublicArtHandler(&fakePublicArtSearchService{}, settings)
	body := `{"candidate":{"provider":"aic","id":"aic:1","title":"Water Lilies","image_url":"https://example.test/image.jpg"},"composition":{"scale_mode":"cover","zoom":1.5,"pan_x":0.1,"pan_y":0,"background_color":"white"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/public-art/select", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	if err := h.Select(e.NewContext(req, rec)); err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var artwork publicart.SelectedArtwork
	if err := json.Unmarshal([]byte(settings.values[publicart.SettingsKeySelectedArtwork]), &artwork); err != nil {
		t.Fatalf("decode stored artwork: %v", err)
	}
	if artwork.Candidate.ID != "aic:1" || artwork.Composition.ScaleMode != "cover" || artwork.Composition.Zoom != 1.5 {
		t.Fatalf("stored artwork = %#v", artwork)
	}
}

func TestPublicArtSelectRejectsMissingImageURL(t *testing.T) {
	e := echo.New()
	h := NewPublicArtHandler(&fakePublicArtSearchService{}, &fakePublicArtSettingsStore{})
	body := `{"candidate":{"provider":"aic","id":"aic:1"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/public-art/select", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	if err := h.Select(e.NewContext(req, rec)); err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestPublicArtThumbnailFallsBackToThumbnailDataURL(t *testing.T) {
	e := echo.New()
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "cloudflare challenge", http.StatusForbidden)
	}))
	defer imageServer.Close()

	h := NewPublicArtHandler(&fakePublicArtSearchService{})
	thumbnail := "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw=="
	req := httptest.NewRequest(http.MethodGet, "/api/public-art/thumbnail?candidate_image_url="+url.QueryEscape(imageServer.URL)+"&candidate_thumbnail_url="+url.QueryEscape(thumbnail), nil)
	rec := httptest.NewRecorder()

	if err := h.Thumbnail(e.NewContext(req, rec)); err != nil {
		t.Fatalf("Thumbnail returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want image/jpeg", got)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("thumbnail response body is empty")
	}
}

func TestPublicArtClearSelectionClearsSetting(t *testing.T) {
	e := echo.New()
	settings := &fakePublicArtSettingsStore{values: map[string]string{publicart.SettingsKeySelectedArtwork: `{"candidate":{"provider":"aic","id":"aic:1","image_url":"https://example.test/img.jpg"},"composition":{"scale_mode":"cover"}}`}}
	h := NewPublicArtHandler(&fakePublicArtSearchService{}, settings)
	req := httptest.NewRequest(http.MethodDelete, "/api/public-art/select", nil)
	rec := httptest.NewRecorder()

	if err := h.ClearSelection(e.NewContext(req, rec)); err != nil {
		t.Fatalf("ClearSelection returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := settings.values[publicart.SettingsKeySelectedArtwork]; got != "" {
		t.Fatalf("selected setting = %q, want empty", got)
	}
}
