package validation

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	log "github.com/golang/glog"
	"io/ioutil"
	"net/http"
	"strings"
)

func addDigest(request *http.Request) (*http.Request, error) {
	if request.Body == nil {
		return request, nil
	}
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body from Request: got err=%v", err)
	}
	hash := sha256.Sum256(body)
	digest := base64.StdEncoding.EncodeToString(hash[:])
	request.Header.Set("Digest", fmt.Sprintf("sha-256=%s", digest))
	request.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	return request, nil
}

// Digest validates the "Digest" header.
func Digest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			log.Errorf("Request body must not be nil")
			http.Error(w, fmt.Sprintf("failed to validate digest header"), http.StatusBadRequest)
			return
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Errorf("failed to read body from Request: got err=%v", err)
			http.Error(w, fmt.Sprintf("failed to validate digest header"), http.StatusBadRequest)
			return
		}
		digest := r.Header.Get("Digest")
		if digest == "" {
			log.Errorf("failed to load Digest: got Digest=%q", digest)
			http.Error(w, fmt.Sprintf("failed to validate digest header"), http.StatusBadRequest)
			return
		}
		var hashUsed, encodedHash string
		splitDigest := strings.SplitN(digest, "=", 2)
		if len(splitDigest) != 2 {
			log.Errorf("failed to parse Digest: got=%q", digest)
			http.Error(w, fmt.Sprintf("failed to validate digest header"), http.StatusBadRequest)
			return
		}
		hashUsed = splitDigest[0]
		encodedHash = splitDigest[1]
		hashReceived, err := base64.StdEncoding.DecodeString(encodedHash)
		if err != nil {
			log.Errorf("failed to decode string=%q: got err=%v", encodedHash, err)
			http.Error(w, fmt.Sprintf("failed to validate digest header"), http.StatusBadRequest)
			return
		}
		var hashCalculated []byte
		switch strings.ToLower(hashUsed) {
		case "sha-256":
			h := sha256.Sum256(body)
			hashCalculated = h[:]
		default:
			log.Errorf("unknown hash function specified: got=%q", hashUsed)
			http.Error(w, fmt.Sprintf("failed to validate digest header"), http.StatusBadRequest)
			return
		}
		if bytes.Compare(hashCalculated, hashReceived) != 0 {
			log.Errorf("hash calculated is different to the hash received")
			http.Error(w, fmt.Sprintf("failed to validate digest header"), http.StatusBadRequest)
			return
		}
		r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		next.ServeHTTP(w, r)
	})
}
