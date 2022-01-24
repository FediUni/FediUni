package validation

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	log "github.com/golang/glog"
	"io/ioutil"
	"net/http"
)

func CalculateDigestHeader(req *http.Request) (*http.Request, error) {
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(body)
	log.Infof("SHA-256=%s", base64.StdEncoding.EncodeToString(h[:]))
	req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	return req, nil
}
