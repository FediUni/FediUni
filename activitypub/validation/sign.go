package validation

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"github.com/go-fed/httpsig"
	log "github.com/golang/glog"
	"net/http"
	"net/url"
	"strings"
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
	headersToSign := []string{httpsig.RequestTarget, "host", "date", "digest"}
	headerPairs := []string{}
	for _, header := range headersToSign {
		switch headerName := strings.ToLower(header); headerName {
		case "(request-target)":
			headerPairs = append(headerPairs, fmt.Sprintf("%s: %s %s", headerName, strings.ToLower(r.Method), r.URL.RequestURI()))
		default:
			headerPairs = append(headerPairs, fmt.Sprintf("%s: %s", headerName, r.Header.Get(headerName)))
		}
	}
	toSign := strings.Join(headerPairs, "\n")
	log.Infof("Signing %q", toSign)
	if err != nil {
		return nil, err
	}
	headers := strings.Join(headersToSign, " ")
	hash := sha256.Sum256([]byte(toSign))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to generate Signature: got err=%v", err)
	}
	encodedSignature := base64.StdEncoding.EncodeToString(signature)
	log.Infof("Determined Signature=%q", encodedSignature)
	signatureHeaderValue := fmt.Sprintf("keyId=%q,algorithm=%q,headers=%q,signature=%q", keyID, "rsa-sha256", headers, encodedSignature)
	log.Infof("Setting Signature Header value: Signature: %s", signatureHeaderValue)
	r.Header.Set("Signature", signatureHeaderValue)
	log.Infof("Request Headers: %v", r.Header)
	return r, nil
}
