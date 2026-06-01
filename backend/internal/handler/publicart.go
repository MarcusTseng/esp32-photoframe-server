package handler

import (
	"bytes"
	"image"
	"io"
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
	Candidate   publicart.Candidate   `json:"candidate"`
	Composition publicart.Composition `json:"composition"`
}

type PublicArtPreviewRequest struct {
	Candidate   publicart.Candidate   `json:"candidate"`
	Composition publicart.Composition `json:"composition"`
	TargetWidth  int                  `json:"target_width"`
	TargetHeight int                  `json:"target_height"`
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
	comp := req.Composition
	if comp.ScaleMode == "" {
		comp = publicart.DefaultComposition()
	}
	artwork := publicart.SelectedArtwork{Candidate: req.Candidate, Composition: comp}
	if err := publicart.SaveSelectedArtwork(h.settings, artwork); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "selected"})
}

func (h *PublicArtHandler) ClearSelection(c echo.Context) error {
	if h.settings == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Public art settings store is not configured"})
	}
	if err := publicart.ClearSelectedArtwork(h.settings); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "cleared"})
}

func (h *PublicArtHandler) Preview(c echo.Context) error {
	var req PublicArtPreviewRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}
	if req.Candidate.ImageURL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "candidate image_url is required"})
	}
	comp := req.Composition
	if comp.ScaleMode == "" {
		comp = publicart.DefaultComposition()
	}
	if req.TargetWidth <= 0 {
		req.TargetWidth = 800
	}
	if req.TargetHeight <= 0 {
		req.TargetHeight = 600
	}
	// Clamp preview size for performance
	if req.TargetWidth > 400 {
		req.TargetWidth = 400
	}
	if req.TargetHeight > 400 {
		req.TargetHeight = 400
	}

	data, err := h.downloadImage(req.Candidate.ImageURL)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to fetch image: " + err.Error()})
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to decode image: " + err.Error()})
	}
	composed := publicart.ComposeImage(img, comp, req.TargetWidth, req.TargetHeight)

	c.Response().Header().Set("Content-Type", "image/jpeg")
	c.Response().WriteHeader(http.StatusOK)
	if err := publicart.EncodeImage(c.Response().Writer, composed, "jpeg"); err != nil {
		return err
	}
	return nil
}

func (h *PublicArtHandler) downloadImage(imageURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	setBrowserLikeHeaders(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, echo.NewHTTPError(resp.StatusCode, "image status "+string(rune(resp.StatusCode)))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 20<<20))
}

func setBrowserLikeHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; esp32-photoframe-server/1.0)")
	req.Header.Set("Accept", "image/webp,image/apng,image/*,*/*;q=0.8")
}
