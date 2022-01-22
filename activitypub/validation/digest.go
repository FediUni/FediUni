package validation

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
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
	request.Header.Set("Digest", fmt.Sprintf("SHA-256=%s", digest))
	return request, nil
}
