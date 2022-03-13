package validation

import (
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-fed/httpsig"
	log "github.com/golang/glog"
)

func SignRequestWithDigest(r *http.Request, url *url.URL, keyID string, privateKey *rsa.PrivateKey, body []byte) (*http.Request, error) {
	if url == nil {
		return nil, fmt.Errorf("failed to receive the Instance url: got=%q", url.String())
	}
	if privateKey == nil {
		return nil, fmt.Errorf("failed to receive a Private Key: got=%v", privateKey)
	}
	if keyID == "" {
		return nil, fmt.Errorf("failed to receive a KeyID: got=%q", keyID)
	}
	if body == nil || len(body) == 0 {
		log.Infoln("No body for request provided")
		return nil, fmt.Errorf("failed to receive body: got=%v", body)
	}
	httpDate := time.Now().UTC().Format(http.TimeFormat)
	r.Header.Add("host", r.Host)
	r.Header.Add("date", httpDate)
	preferences := []httpsig.Algorithm{httpsig.RSA_SHA256, httpsig.RSA_SHA512}
	headersToSign := []string{httpsig.RequestTarget, "host", "date", "digest"}
	signer, _, err := httpsig.NewSigner(preferences, httpsig.DigestSha256, headersToSign, httpsig.Signature, 0)

	// See: https://github.com/go-fed/httpsig/issues/20
	// This is because certain implementations such as Plemora rely on the
	// algorithm name.
	r.Header["Signature"][0] = strings.Replace(r.Header["Signature"][0], "algorithm=\"hs2019\"", "algorithm=\"rsa-sha256\"", 1)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: got err=%v", err)
	}
	return r, signer.SignRequest(privateKey, keyID, r, body)
}
