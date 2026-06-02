package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"testing"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func onePixelPNGBase64(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestAIGenerationMiniMaxGlobalSendsExpectedRequest(t *testing.T) {
	settings := NewSettingsService(setupTestDB())
	if err := settings.Set("minimax_global_api_key", "test-global-key"); err != nil {
		t.Fatalf("set api key: %v", err)
	}

	var seenURL string
	var seenAuth string
	var payload map[string]interface{}
	svc := NewAIGenerationService(settings)
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seenURL = req.URL.String()
		seenAuth = req.Header.Get("Authorization")
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			return nil, err
		}
		body := `{"data":{"image_base64":["` + onePixelPNGBase64(t) + `"]}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Request:    req,
		}, nil
	})}

	img, err := svc.Generate(&model.Device{
		Name:       "frame",
		Width:      800,
		Height:     480,
		AIProvider: "minimax_global",
		AIModel:    "image-01",
		AIPrompt:   "a watercolor landscape",
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if img == nil {
		t.Fatal("Generate returned nil image")
	}
	if seenURL != "https://api.minimax.io/v1/image_generation" {
		t.Fatalf("unexpected URL: %s", seenURL)
	}
	if seenAuth != "Bearer test-global-key" {
		t.Fatalf("unexpected auth header: %s", seenAuth)
	}
	if payload["model"] != "image-01" || payload["prompt"] != "a watercolor landscape" || payload["aspect_ratio"] != "16:9" || payload["response_format"] != "base64" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestAIGenerationMiniMaxChinaUsesChinaEndpointAndPortraitAspect(t *testing.T) {
	settings := NewSettingsService(setupTestDB())
	if err := settings.Set("minimax_china_api_key", "test-cn-key"); err != nil {
		t.Fatalf("set api key: %v", err)
	}

	var seenURL string
	var payload map[string]interface{}
	svc := NewAIGenerationService(settings)
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seenURL = req.URL.String()
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			return nil, err
		}
		body := `{"data":{"image_base64":["` + onePixelPNGBase64(t) + `"]}}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(body)), Request: req}, nil
	})}

	_, err := svc.Generate(&model.Device{
		Name:        "portrait-frame",
		Width:       480,
		Height:      800,
		Orientation: "portrait",
		AIProvider:  "minimax_china",
		AIModel:     "image-01",
		AIPrompt:    "ink painting",
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if seenURL != "https://api.minimaxi.com/v1/image_generation" {
		t.Fatalf("unexpected URL: %s", seenURL)
	}
	if payload["aspect_ratio"] != "9:16" {
		t.Fatalf("unexpected aspect ratio payload: %#v", payload)
	}
}
