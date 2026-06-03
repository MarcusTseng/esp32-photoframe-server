package publicart

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultAICBaseURL = "https://api.artic.edu"

type AICProvider struct {
	baseURL string
	client  *http.Client
}

func NewAICProvider(baseURL string, client *http.Client) *AICProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultAICBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &AICProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  client,
	}
}

func (p *AICProvider) Search(query string, opts SearchOptions) ([]Candidate, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("publicart: aic search query is required")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	u, err := url.Parse(p.baseURL + "/api/v1/artworks/search")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("fields", "id,title,artist_title,date_display,image_id,thumbnail")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	setBrowserLikeHeaders(req)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("publicart: aic search request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("publicart: aic search status %d", resp.StatusCode)
	}

	var result aicSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("publicart: decode aic response: %w", err)
	}
	return result.Candidates(), nil
}

type aicSearchResponse struct {
	Config struct {
		IIIFURL string `json:"iiif_url"`
	} `json:"config"`
	Data []struct {
		ID          int     `json:"id"`
		Title       string  `json:"title"`
		ArtistTitle string  `json:"artist_title"`
		DateDisplay string  `json:"date_display"`
		ImageID     *string `json:"image_id"`
		Thumbnail   struct {
			LQIP   string `json:"lqip"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"thumbnail"`
	} `json:"data"`
}

func (r aicSearchResponse) Candidates() []Candidate {
	iiifURL := strings.TrimRight(r.Config.IIIFURL, "/")
	if iiifURL == "" {
		iiifURL = "https://www.artic.edu/iiif/2"
	}

	candidates := make([]Candidate, 0, len(r.Data))
	for _, item := range r.Data {
		if item.ImageID == nil || strings.TrimSpace(*item.ImageID) == "" {
			continue
		}
		imageID := strings.TrimSpace(*item.ImageID)
		thumbnailURL := strings.TrimSpace(item.Thumbnail.LQIP)
		if thumbnailURL == "" {
			thumbnailURL = fmt.Sprintf("%s/%s/full/600,/0/default.jpg", iiifURL, url.PathEscape(imageID))
		}
		candidates = append(candidates, Candidate{
			Provider:     ProviderAIC,
			ID:           fmt.Sprintf("aic:%d", item.ID),
			Title:        item.Title,
			Artist:       item.ArtistTitle,
			Date:         item.DateDisplay,
			ImageURL:     fmt.Sprintf("%s/%s/full/2000,/0/default.jpg", iiifURL, url.PathEscape(imageID)),
			ThumbnailURL: thumbnailURL,
			SourceURL:    fmt.Sprintf("https://www.artic.edu/artworks/%d", item.ID),
			Width:        item.Thumbnail.Width,
			Height:       item.Thumbnail.Height,
		})
	}
	return candidates
}

// TryImgixFallback converts an AIC IIIF URL to an imgix CDN proxy URL.
// The imgix proxy (https://img.artic.edu) is reachable from environments
// that can't reach the raw IIIF endpoint (e.g., HA addon containers with
// strict outbound routing). Returns empty string if URL is not an AIC IIIF URL.
func TryImgixFallback(iiifURL string) string {
	const imgixBase = "https://img.artic.edu"
	if iiifURL == "" || !strings.Contains(iiifURL, "artic.edu/iiif") {
		return ""
	}
	// Replace the IIIF host with the imgix proxy while preserving path and params.
	// Original: https://www.artic.edu/iiif/2/{id}/full/600,/0/default.jpg
	// Proxied:  https://img.artic.edu/iiif/2/{id}/full/600,/0/default.jpg
	idx := strings.Index(iiifURL, "artic.edu/iiif")
	path := iiifURL[idx+len("artic.edu"):]
	return imgixBase + path
}
