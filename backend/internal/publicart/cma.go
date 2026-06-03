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

const defaultCMABaseURL = "https://openaccess-api.clevelandart.org"

type CMAProvider struct {
	baseURL string
	client  *http.Client
}

func NewCMAProvider(baseURL string, client *http.Client) *CMAProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultCMABaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &CMAProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  client,
	}
}

func (p *CMAProvider) Search(query string, opts SearchOptions) ([]Candidate, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("publicart: cma search query is required")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	u, err := url.Parse(p.baseURL + "/api/artworks/")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("has_image", "1")
	q.Set("limit", strconv.Itoa(limit))
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	setBrowserLikeHeaders(req)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("publicart: cma search request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("publicart: cma search status %d", resp.StatusCode)
	}

	var result cmaSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("publicart: decode cma response: %w", err)
	}
	return result.Candidates(), nil
}

type cmaSearchResponse struct {
	Data []struct {
		ID           int    `json:"id"`
		Title        string `json:"title"`
		CreationDate string `json:"creation_date"`
		URL          string `json:"url"`
		Creators     []struct {
			Description string `json:"description"`
		} `json:"creators"`
		Images struct {
			Web   cmaImage `json:"web"`
			Print cmaImage `json:"print"`
		} `json:"images"`
	} `json:"data"`
}

type cmaImage struct {
	URL    string `json:"url"`
	Width  string `json:"width"`
	Height string `json:"height"`
}

func (r cmaSearchResponse) Candidates() []Candidate {
	candidates := make([]Candidate, 0, len(r.Data))
	for _, item := range r.Data {
		imageURL := strings.TrimSpace(item.Images.Print.URL)
		if imageURL == "" {
			imageURL = strings.TrimSpace(item.Images.Web.URL)
		}
		thumbnailURL := strings.TrimSpace(item.Images.Web.URL)
		if thumbnailURL == "" {
			thumbnailURL = imageURL
		}
		if imageURL == "" {
			continue
		}
		width, height := parseCMAImageSize(item.Images.Print)
		if width == 0 || height == 0 {
			width, height = parseCMAImageSize(item.Images.Web)
		}
		artist := ""
		if len(item.Creators) > 0 {
			artist = item.Creators[0].Description
		}
		candidates = append(candidates, Candidate{
			Provider:     ProviderCMA,
			ID:           fmt.Sprintf("cma:%d", item.ID),
			Title:        item.Title,
			Artist:       artist,
			Date:         item.CreationDate,
			ImageURL:     imageURL,
			ThumbnailURL: thumbnailURL,
			SourceURL:    item.URL,
			Width:        width,
			Height:       height,
		})
	}
	return candidates
}

func parseCMAImageSize(img cmaImage) (int, int) {
	w, _ := strconv.Atoi(strings.TrimSpace(img.Width))
	h, _ := strconv.Atoi(strings.TrimSpace(img.Height))
	return w, h
}
