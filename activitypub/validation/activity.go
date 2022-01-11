package validation

import (
	"fmt"
	"net/http"
	"strings"

	log "github.com/golang/glog"
)

func Activity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch contentType := r.Header.Get("Content-Type"); {
		case strings.HasPrefix(contentType, "application/ld+json"):
		case strings.HasPrefix(contentType, "application/activity+json"):
		default:
			log.Errorf("unexpected Content-Type presented: got Content-Type=%q", contentType)
			http.Error(w, fmt.Sprintf("unexpected Content-Type presented"), http.StatusBadRequest)
		}
		next.ServeHTTP(w, r)
	})
}
