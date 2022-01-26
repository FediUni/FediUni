package validation

import (
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	log "github.com/golang/glog"
	"github.com/spacemonkeygo/httpsig"
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
	r, err := CalculateDigestHeader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate Digest header: got err=%v", err)
	}
	headersToSign := []string{"(request-target)", "host", "date", "digest"}
	signer := httpsig.NewRSASHA256Signer(keyID, privateKey, headersToSign)
	if err := signer.Sign(r); err != nil {
		return nil, fmt.Errorf("failed to sign request: got err=%v", err)
	}
	authorizationHeader := r.Header.Get("Authorization")
	log.Infof("Authorization=%q", authorizationHeader)
	splitHeader := strings.SplitN(authorizationHeader, " ", 2)
	r.Header.Set("Signature", splitHeader[1])
	log.Infof("Signature=%q", splitHeader[1])
	r.Header.Del("Authorization")
	return r, nil
}
