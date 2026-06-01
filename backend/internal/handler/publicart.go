package handler

import (
	"bytes"
	"image"
	"io"
	"net/http"
	"strconv"

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

// PreviewImageRequest is used for GET /public-art/preview with query params.
type PreviewImageRequest struct {
	CandidateImageURL string  `query:"candidate_image_url"`
	ScaleMode         string  `query:"scale_mode"`
	Zoom              float64 `query:"zoom"`
	PanX              float64 `query:"pan_x"`
	PanY              float64 `query:"pan_y"`
	BackgroundColor   string  `query:"background_color"`
	TargetWidth       int     `query:"target_width"`
	TargetHeight      int     `query:"target_height"`
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
	// Support GET (query params) and POST (JSON body)
	var comp publicart.Composition
	var imageURL string
	var targetW, targetH int

	if c.Request().Method == http.MethodGet {
		// GET /public-art/preview?candidate_image_url=...&scale_mode=cover&...
		imageURL = c.QueryParam("candidate_image_url")
		if imageURL == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "candidate_image_url is required"})
		}
		comp.ScaleMode = c.QueryParam("scale_mode")
		if comp.ScaleMode == "" {
			comp = publicart.DefaultComposition()
		}
		if v := c.QueryParam("zoom"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				comp.Zoom = f
			}
		}
		if v := c.QueryParam("pan_x"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				comp.PanX = f
			}
		}
		if v := c.QueryParam("pan_y"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				comp.PanY = f
			}
		}
		comp.BackgroundColor = c.QueryParam("background_color")
		if v := c.QueryParam("target_width"); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				targetW = i
			}
		}
		if v := c.QueryParam("target_height"); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				targetH = i
			}
		}
	} else {
		// POST with JSON body
		var req PublicArtPreviewRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		}
		imageURL = req.Candidate.ImageURL
		if imageURL == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "candidate image_url is required"})
		}
		comp = req.Composition
		if comp.ScaleMode == "" {
			comp = publicart.DefaultComposition()
		}
		targetW = req.TargetWidth
		targetH = req.TargetHeight
	}

	// Clamp preview size for performance
	if targetW <= 0 {
		targetW = 400
	} else if targetW > 400 {
		targetW = 400
	}
	if targetH <= 0 {
		targetH = 300
	} else if targetH > 400 {
		targetH = 400
	}

	data, err := h.downloadImage(imageURL)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to fetch image: " + err.Error()})
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to decode image: " + err.Error()})
	}
	composed := publicart.ComposeImage(img, comp, targetW, targetH)

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
