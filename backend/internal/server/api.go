package server

import (
	"net/http"
	"strings"
)

func APIMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setAPIHeaders(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func setAPIHeaders(w http.ResponseWriter, r *http.Request) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return
	}

	headers := w.Header()
	addVaryHeader(headers, "Origin")
	addVaryHeader(headers, "Access-Control-Request-Method")
	addVaryHeader(headers, "Access-Control-Request-Headers")
	headers.Set("Access-Control-Allow-Origin", origin)
	headers.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	headers.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	headers.Set("Access-Control-Max-Age", "600")
}

func addVaryHeader(headers http.Header, value string) {
	existing := headers.Values("Vary")
	for _, entry := range existing {
		for _, part := range strings.Split(entry, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}

	headers.Add("Vary", value)
}
