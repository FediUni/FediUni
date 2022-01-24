package validation

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
)

func CalculateDigestHeader(req *http.Request) (*http.Request, error) {
	if req.Body == nil {
		return nil, nil
	}
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(body)
	digest := fmt.Sprintf("SHA-256=%s", base64.StdEncoding.EncodeToString(h[:]))
	req.Header.Set("Digest", digest)
	req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	return req, nil
}
