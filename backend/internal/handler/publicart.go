package handler

import (
	"net/http"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/publicart"
	"github.com/labstack/echo/v4"
)

type publicArtSearcher interface {
	SearchCandidates(cfg publicart.Config, limit int) ([]publicart.Candidate, error)
}

type PublicArtHandler struct {
	service  publicArtSearcher
	settings publicart.SettingsStore
}

func NewPublicArtHandler(service publicArtSearcher, settings ...publicart.SettingsStore) *PublicArtHandler {
	h := &PublicArtHandler{service: service}
	if len(settings) > 0 {
		h.settings = settings[0]
	}
	return h
}

type PublicArtSearchRequest struct {
	Provider               string `json:"provider"`
	Query                  string `json:"query"`
	MinImageLongEdge       int    `json:"min_image_long_edge"`
	PreferredImageLongEdge int    `json:"preferred_image_long_edge"`
	Limit                  int    `json:"limit"`
}

type PublicArtSelectRequest struct {
	Candidate publicart.Candidate `json:"candidate"`
}

func (h *PublicArtHandler) Search(c echo.Context) error {
	var req PublicArtSearchRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}
	candidates, err := h.service.SearchCandidates(publicart.Config{
		Provider:               req.Provider,
		Query:                  req.Query,
		MinImageLongEdge:       req.MinImageLongEdge,
		PreferredImageLongEdge: req.PreferredImageLongEdge,
	}, limit)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, candidates)
}

func (h *PublicArtHandler) Select(c echo.Context) error {
	if h.settings == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Public art settings store is not configured"})
	}
	var req PublicArtSelectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}
	if err := publicart.SaveSelectedCandidate(h.settings, req.Candidate); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "selected"})
}

func (h *PublicArtHandler) ClearSelection(c echo.Context) error {
	if h.settings == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Public art settings store is not configured"})
	}
	if err := publicart.ClearSelectedCandidate(h.settings); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "cleared"})
}
