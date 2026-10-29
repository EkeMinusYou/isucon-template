package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
)

// setMeasurementHeaders runs before the response headers are committed.
// Bootstrap supplies an opaque session identity after authentication and, if
// known, the bytes actually read from the request. No cookie name is assumed.
// nginx records these values and hides them from the client response.
func setMeasurementHeaders(w http.ResponseWriter, sessionIdentity string, bodyBytes *int64) {
	header := w.Header()
	header.Del("X-Measurement-Session-ID")
	if sessionIdentity != "" {
		digest := sha256.Sum256([]byte(sessionIdentity))
		header.Set("X-Measurement-Session-ID", hex.EncodeToString(digest[:]))
	}
	header.Del("X-Measurement-Request-Body-Bytes")
	if bodyBytes != nil && *bodyBytes >= 0 {
		header.Set("X-Measurement-Request-Body-Bytes", strconv.FormatInt(*bodyBytes, 10))
	}
}
