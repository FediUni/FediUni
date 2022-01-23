package validation

import (
	"crypto/rsa"
	"github.com/go-fed/httpsig"
	"net/http"
	"net/url"
	"time"
)

func SignRequestWithDigest(r *http.Request, url *url.URL, keyID string, privateKey *rsa.PrivateKey, body []byte) (*http.Request, error) {
	httpDate := time.Now().UTC().Format(http.TimeFormat)
	host := url.Host
	r.Header.Set("Host", host)
	r.Header.Set("Date", httpDate)
	prefs := []httpsig.Algorithm{httpsig.RSA_SHA256}
	// The "Date" and "Digest" headers must already be set on r, as well as r.URL.
	headersToSign := []string{httpsig.RequestTarget, "host", "date", "digest"}
	signer, _, err := httpsig.NewSigner(prefs, httpsig.DigestSha256, headersToSign, httpsig.Signature)
	if err != nil {
		return nil, err
	}
	if err := signer.SignRequest(privateKey, keyID, r, body); err != nil {
		return nil, err
	}
	return r, nil
}
