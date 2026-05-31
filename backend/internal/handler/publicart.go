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
	service publicArtSearcher
}

func NewPublicArtHandler(service publicArtSearcher) *PublicArtHandler {
	return &PublicArtHandler{service: service}
}

type PublicArtSearchRequest struct {
	Provider               string `json:"provider"`
	Query                  string `json:"query"`
	MinImageLongEdge       int    `json:"min_image_long_edge"`
	PreferredImageLongEdge int    `json:"preferred_image_long_edge"`
	Limit                  int    `json:"limit"`
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
