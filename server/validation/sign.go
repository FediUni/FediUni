package validation

import (
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/url"
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
	}
	httpDate := time.Now().UTC().Format(http.TimeFormat)
	host := url.Host
	r.Header.Set("Host", host)
	r.Header.Set("Date", httpDate)
	preferences := []httpsig.Algorithm{httpsig.RSA_SHA256}
	headersToSign := []string{"(request-target)", "host", "date", "digest"}
	signer, _, err := httpsig.NewSigner(preferences, httpsig.DigestSha256, headersToSign, httpsig.Signature)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: got err=%v", err)
	}
	return r, signer.SignRequest(privateKey, keyID, r, body)
}
