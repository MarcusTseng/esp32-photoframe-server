package publicart

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"strings"
)

// ComposeImage composes src onto a newly-allocated image of size targetW×targetH,
// following the given Composition rules. Returns the composed image ready for
// the e-paper pipeline.
func ComposeImage(src image.Image, comp Composition, targetW, targetH int) image.Image {
	if targetW <= 0 {
		targetW = 800
	}
	if targetH <= 0 {
		targetH = 600
	}

	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		// Return blank image of target size
		img := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
		fillRect(img, img.Bounds(), color.White)
		return img
	}

	switch comp.ScaleMode {
	case "fit":
		return composeFit(src, srcW, srcH, targetW, targetH, comp.BackgroundColor)
	default: // "cover" and "custom"
		return composeCover(src, srcW, srcH, targetW, targetH, comp.Zoom, comp.PanX, comp.PanY)
	}
}

// composeCover scales the image to cover the target rectangle with optional zoom and pan.
// The cropped region is centered by default; pan offsets shift the crop window.
func composeCover(src image.Image, srcW, srcH, targetW, targetH int, zoom, panX, panY float64) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	fillRect(img, img.Bounds(), parseColor("white"))

	// Compute aspect ratios
	srcAspect := float64(srcW) / float64(srcH)
	targetAspect := float64(targetW) / float64(targetH)

	var cropW, cropH int
	if srcAspect > targetAspect {
		// Source is wider: fit to height, crop width
		cropH = srcH
		cropW = int(math.Round(float64(srcH) * targetAspect))
	} else {
		// Source is taller: fit to width, crop height
		cropW = srcW
		cropH = int(math.Round(float64(srcW) / targetAspect))
	}

	// Apply zoom (1.0 = no zoom, 2.0 = zoom in 2× = smaller crop)
	if zoom < 0.1 {
		zoom = 1.0
	}
	cropW = int(math.Round(float64(cropW) / zoom))
	cropH = int(math.Round(float64(cropH) / zoom))

	// Clamp crop to source bounds
	if cropW > srcW {
		cropW = srcW
	}
	if cropH > srcH {
		cropH = srcH
	}

	// Compute top-left corner of the crop region
	// Center the crop, then apply pan offset
	cx := (srcW - cropW) / 2
	cy := (srcH - cropH) / 2

	// panX/panY are -0.5 to 0.5, shift by fraction of crop size
	panOffsetX := int(math.Round(panX * float64(cropW)))
	panOffsetY := int(math.Round(panY * float64(cropH)))
	cx += panOffsetX
	cy += panOffsetY

	// Clamp so crop stays within source
	if cx < 0 {
		cx = 0
	} else if cx+cropW > srcW {
		cx = srcW - cropW
	}
	if cy < 0 {
		cy = 0
	} else if cy+cropH > srcH {
		cy = srcH - cropH
	}

	// Scale the cropped region to target size
	scale := float64(targetW) / float64(cropW)
	scaleY := float64(targetH) / float64(cropH)

	// Use nearest-neighbor for simplicity (fast, no extra deps)
	for y := 0; y < targetH; y++ {
		srcY := cy + int(math.Round(float64(y)/scaleY))
		if srcY < 0 {
			srcY = 0
		} else if srcY >= srcH {
			srcY = srcH - 1
		}
		for x := 0; x < targetW; x++ {
			srcX := cx + int(math.Round(float64(x)/scale))
			if srcX < 0 {
				srcX = 0
			} else if srcX >= srcW {
				srcX = srcW - 1
			}
			img.Set(x, y, src.At(srcX, srcY))
		}
	}

	return img
}

// composeFit scales the image to fit entirely within the target rectangle,
// placing it centered and filling remaining space with background color.
func composeFit(src image.Image, srcW, srcH, targetW, targetH int, bgColor string) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	fillRect(img, img.Bounds(), parseColor(bgColor))

	// Compute scale to fit inside target
	scale := math.Min(float64(targetW)/float64(srcW), float64(targetH)/float64(srcH))
	dstW := int(math.Round(float64(srcW) * scale))
	dstH := int(math.Round(float64(srcH) * scale))

	// Center within target
	ox := (targetW - dstW) / 2
	oy := (targetH - dstH) / 2

	for y := 0; y < dstH; y++ {
		srcY := int(math.Round(float64(y) / scale))
		if srcY < 0 {
			srcY = 0
		} else if srcY >= srcH {
			srcY = srcH - 1
		}
		for x := 0; x < dstW; x++ {
			srcX := int(math.Round(float64(x) / scale))
			if srcX < 0 {
				srcX = 0
			} else if srcX >= srcW {
				srcX = srcW - 1
			}
			img.Set(x+ox, y+oy, src.At(srcX, srcY))
		}
	}

	return img
}

// fillRect fills rect with a solid color by setting pixels directly.
func fillRect(img *image.RGBA, rect image.Rectangle, c color.Color) {
	r, g, b, a := c.RGBA()
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.Set(x, y, color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)})
		}
	}
}

// parseColor converts a color name or hex string to a color.RGBA.
func parseColor(s string) color.Color {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "white":
		return color.White
	case "black":
		return color.Black
	case "transparent":
		return color.Transparent
	}
	// Try #rrggbb or #rgb
	if strings.HasPrefix(s, "#") {
		s = s[1:]
		if len(s) == 3 {
			// Expand #rgb → #rrggbb
			s = string(s[0]) + string(s[0]) +
				string(s[1]) + string(s[1]) +
				string(s[2]) + string(s[2])
		}
		if len(s) == 6 {
			var r, g, b uint8
			if _, err := parseHexPair(s[0:2], &r); err == nil {
				_, _ = parseHexPair(s[2:4], &g)
				_, _ = parseHexPair(s[4:6], &b)
				return color.RGBA{R: r, G: g, B: b, A: 255}
			}
		}
	}
	return color.White // default
}

func parseHexPair(s string, out *uint8) (bool, error) {
	var v uint8
	for _, c := range s {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= uint8(c - '0')
		case c >= 'a' && c <= 'f':
			v |= uint8(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			v |= uint8(c - 'A' + 10)
		default:
			return false, nil
		}
	}
	*out = v
	return true, nil
}

// EncodeImage encodes img as JPEG (for preview) or PNG, writing to w.
func EncodeImage(w io.Writer, img image.Image, format string) error {
	switch strings.ToLower(format) {
	case "png":
		return png.Encode(w, img)
	default:
		return jpeg.Encode(w, img, &jpeg.Options{Quality: 85})
	}
}
