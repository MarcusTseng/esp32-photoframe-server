package handler

import (
	"bytes"
	"fmt"
	"image"
	"net/http"
	"os"
	"strconv"
	"strings"

	"io/ioutil"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/publicart"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type DeviceHandler struct {
	deviceService   *service.DeviceService
	synologyService *service.SynologyService
	immichService   *service.ImmichService
	db              *gorm.DB
}

func NewDeviceHandler(deviceService *service.DeviceService, synologyService *service.SynologyService, immichService *service.ImmichService, db *gorm.DB) *DeviceHandler {
	return &DeviceHandler{
		deviceService:   deviceService,
		synologyService: synologyService,
		immichService:   immichService,
		db:              db,
	}
}

// ... existing methods ... (List, Add, Update, Delete, Push)

// GET /api/devices
func (h *DeviceHandler) ListDevices(c echo.Context) error {
	devices, err := h.deviceService.ListDevices()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, devices)
}

// POST /api/devices
func (h *DeviceHandler) AddDevice(c echo.Context) error {
	var req struct {
		Host          string  `json:"host"`
		EnableCollage bool    `json:"enable_collage"`
		ShowDate      bool    `json:"show_date"`
		ShowPhotoDate bool    `json:"show_photo_date"`
		ShowWeather   bool    `json:"show_weather"`
		WeatherLat    float64 `json:"weather_lat"`
		WeatherLon    float64 `json:"weather_lon"`
		Layout        string  `json:"layout"`
		DisplayMode   string  `json:"display_mode"`
		ShowCalendar  bool    `json:"show_calendar"`
		CalendarID    string  `json:"calendar_id"`
		DateFormat    string  `json:"date_format"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.Host == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "host required"})
	}

	if req.Layout == "" {
		req.Layout = model.LayoutPhotoOverlay
	}

	device, err := h.deviceService.AddDevice(req.Host, req.EnableCollage, req.ShowDate, req.ShowPhotoDate, req.ShowWeather, req.WeatherLat, req.WeatherLon, req.Layout, req.DisplayMode, req.ShowCalendar, req.CalendarID, req.DateFormat)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusCreated, device)
}

// PUT /api/devices/:id
// Updates server-owned + shared fields only. Dimensions / board name
// come from POST /api/devices/:id/refresh.
func (h *DeviceHandler) UpdateDevice(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Name          string  `json:"name"`
		Host          string  `json:"host"`
		Orientation   string  `json:"orientation"`
		EnableCollage bool    `json:"enable_collage"`
		ShowDate      bool    `json:"show_date"`
		ShowPhotoDate bool    `json:"show_photo_date"`
		ShowWeather   bool    `json:"show_weather"`
		WeatherLat    float64 `json:"weather_lat"`
		WeatherLon    float64 `json:"weather_lon"`
		AIProvider    string  `json:"ai_provider"`
		AIModel       string  `json:"ai_model"`
		AIPrompt      string  `json:"ai_prompt"`
		Layout        string  `json:"layout"`
		DisplayMode   string  `json:"display_mode"`
		ShowCalendar  bool    `json:"show_calendar"`
		CalendarID    string  `json:"calendar_id"`
		DateFormat    string  `json:"date_format"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.Layout == "" {
		req.Layout = model.LayoutPhotoOverlay
	}

	device, err := h.deviceService.UpdateDevice(uint(id), req.Name, req.Host, req.Orientation, req.EnableCollage, req.ShowDate, req.ShowPhotoDate, req.ShowWeather, req.WeatherLat, req.WeatherLon, req.AIProvider, req.AIModel, req.AIPrompt, req.Layout, req.DisplayMode, req.ShowCalendar, req.CalendarID, req.DateFormat)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, device)
}

// POST /api/devices/:id/refresh
// Pulls dimensions, board name, config, processing settings, and palette
// from the device. Requires the device to be online.
func (h *DeviceHandler) RefreshDevice(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	device, err := h.deviceService.RefreshDeviceFromHardware(uint(id))
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "failed to fetch") {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": errMsg})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errMsg})
	}
	return c.JSON(http.StatusOK, device)
}

// DELETE /api/devices/:id
func (h *DeviceHandler) DeleteDevice(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.deviceService.DeleteDevice(uint(id)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// POST /api/devices/:id/push
func (h *DeviceHandler) PushToDevice(c echo.Context) error {
	deviceID, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		ImageID   uint   `json:"image_id"`
		URL       string `json:"url"` // Optional direct server-side file path
		PublicArt *struct {
			Candidate   publicart.Candidate   `json:"candidate"`
			Composition publicart.Composition `json:"composition"`
		} `json:"public_art"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	imagePath := req.URL
	var tempFile string // If we create a temp file, we must clean it up

	if req.ImageID != 0 {
		var img model.Image
		if err := h.db.First(&img, req.ImageID).Error; err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "image not found"})
		}

		if img.Source == model.SourceSynologyPhotos {
			// Download to temporary file
			data, err := h.synologyService.DownloadPhoto(int(img.SynologyPhotoID))
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to download synology photo: %v", err)})
			}

			// Save to temp file
			tmp, err := ioutil.TempFile("", "syno_push_*.jpg")
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create temp file"})
			}
			defer os.Remove(tmp.Name()) // Clean up
			tempFile = tmp.Name()

			if _, err := tmp.Write(data); err != nil {
				tmp.Close()
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write temp file"})
			}
			tmp.Close()
			imagePath = tempFile
		} else if img.Source == model.SourceImmich {
			// Download from Immich to temporary file
			data, err := h.immichService.DownloadPhoto(img.ImmichAssetID)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to download immich photo: %v", err)})
			}

			tmp, err := ioutil.TempFile("", "immich_push_*.jpg")
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create temp file"})
			}
			defer os.Remove(tmp.Name())
			tempFile = tmp.Name()

			if _, err := tmp.Write(data); err != nil {
				tmp.Close()
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write temp file"})
			}
			tmp.Close()
			imagePath = tempFile
		} else {
			imagePath = img.FilePath
		}
	} else if req.PublicArt != nil {
		composedPath, err := h.composePublicArtForDevice(
			uint(deviceID),
			req.PublicArt.Candidate,
			req.PublicArt.Composition,
		)
		if err != nil {
			return c.JSON(http.StatusBadGateway, map[string]string{"error": err.Error()})
		}
		defer os.Remove(composedPath)
		tempFile = composedPath
		imagePath = tempFile
	}

	if imagePath == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "image path or id required"})
	}

	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "image file not found on server"})
	}

	// Push
	if err := h.deviceService.PushToDevice(uint(deviceID), imagePath); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not reachable") || strings.Contains(errMsg, "failed to resolve") {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"error": "Device is not reachable. Please ensure the device is online and accessible.",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("push failed: %v", err)})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "pushed"})
}

func (h *DeviceHandler) composePublicArtForDevice(deviceID uint, candidate publicart.Candidate, comp publicart.Composition) (string, error) {
	if candidate.ImageURL == "" {
		return "", fmt.Errorf("public art image_url is required")
	}

	var device model.Device
	if err := h.db.First(&device, deviceID).Error; err != nil {
		return "", fmt.Errorf("device not found")
	}

	targetW, targetH := device.Width, device.Height
	if targetW <= 0 || targetH <= 0 {
		targetW, targetH = 800, 480
	}
	if device.Orientation == "portrait" && targetW > targetH {
		targetW, targetH = targetH, targetW
	} else if device.Orientation == "landscape" && targetW < targetH {
		targetW, targetH = targetH, targetW
	}

	data, err := downloadPublicArtImage(candidate.ImageURL, candidate.ThumbnailURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch public art image: %w", err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to decode public art image: %w", err)
	}
	if comp.ScaleMode == "" {
		comp = publicart.DefaultComposition()
	}
	composed := publicart.ComposeImage(img, comp, targetW, targetH)

	tmp, err := ioutil.TempFile("", "public_art_push_*.png")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file")
	}
	if err := publicart.EncodeImage(tmp, composed, "png"); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("failed to encode public art image: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}
	return tmp.Name(), nil
}

func downloadPublicArtImage(primaryURL, fallbackURL string) ([]byte, error) {
	if data, err := downloadPublicHTTPImage(primaryURL); err == nil {
		return data, nil
	}
	if imgixURL := publicart.TryImgixFallback(primaryURL); imgixURL != "" {
		if data, err := downloadPublicHTTPImage(imgixURL); err == nil {
			return data, nil
		}
	}
	if fallbackURL != "" && fallbackURL != primaryURL {
		if data, err := downloadPublicHTTPImage(fallbackURL); err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("all public art image URLs failed")
}

func downloadPublicHTTPImage(url string) ([]byte, error) {
	if url == "" {
		return nil, fmt.Errorf("empty URL")
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return ioutil.ReadAll(resp.Body)
}
