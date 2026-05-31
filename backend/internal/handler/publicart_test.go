package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestPublicArtSearchReturnsRankedCandidates(t *testing.T) {
	e := echo.New()
	svc := &fakePublicArtSearchService{}
	h := NewPublicArtHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/public-art/search", strings.NewReader(`{"query":"monet","limit":5}`))
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
