package validation

import (
	"crypto/rsa"
	"fmt"
	"github.com/go-fed/httpsig"
	"net/http"
	"net/url"
	"time"
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
	httpDate := time.Now().UTC().Format(http.TimeFormat)
	host := url.Host
	r.Header.Set("Host", host)
	r.Header.Set("Date", httpDate)
	prefs := []httpsig.Algorithm{httpsig.RSA_SHA256}
	// The "Date" and "Digest" headers must already be set on r, as well as r.URL.
	headersToSign := []string{httpsig.RequestTarget, "Host", "Date", "Digest"}
	signer, _, err := httpsig.NewSigner(prefs, httpsig.DigestSha256, headersToSign, httpsig.Signature)
	if err != nil {
		return nil, err
	}
	if err := signer.SignRequest(privateKey, keyID, r, body); err != nil {
		return nil, err
	}
	return r, nil
}
