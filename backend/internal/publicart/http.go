package publicart

import "net/http"

func setBrowserLikeHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; esp32-photoframe-server public-art)")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	if req.URL != nil && req.URL.Host == "www.artic.edu" {
		req.Header.Set("Referer", "https://www.artic.edu/")
	}
}
