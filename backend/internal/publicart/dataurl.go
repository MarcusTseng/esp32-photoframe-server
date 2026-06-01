package publicart

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

// DecodeDataImageURL decodes a data:image/... URL. The bool return is false
// when the input is not a data URL, letting callers fall back to HTTP.
func DecodeDataImageURL(raw string) ([]byte, bool, error) {
	if !strings.HasPrefix(raw, "data:") {
		return nil, false, nil
	}
	meta, payload, ok := strings.Cut(raw, ",")
	if !ok {
		return nil, true, fmt.Errorf("publicart: malformed data URL")
	}
	if !strings.HasPrefix(meta, "data:image/") {
		return nil, true, fmt.Errorf("publicart: data URL is not an image")
	}
	if strings.HasSuffix(strings.ToLower(meta), ";base64") || strings.Contains(strings.ToLower(meta), ";base64;") {
		data, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, true, fmt.Errorf("publicart: decode data URL: %w", err)
		}
		return data, true, nil
	}
	data, err := url.QueryUnescape(payload)
	if err != nil {
		return nil, true, fmt.Errorf("publicart: decode data URL: %w", err)
	}
	return []byte(data), true, nil
}
