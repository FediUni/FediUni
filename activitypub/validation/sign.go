package validation

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	log "github.com/golang/glog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func SignRequestWithDigest(r *http.Request, url *url.URL, keyID string, privateKey *rsa.PrivateKey) (*http.Request, error) {
	if keyID == "" {
		return nil, fmt.Errorf("keyID must be specified, got=%q", keyID)
	}
	if url.Host == "" {
		return nil, fmt.Errorf("URL host must be specifed, got=%q", url.String())
	}
	log.Infoln("Calculating Digest...")
	r, err := addDigest(r)
	if err != nil {
		return nil, err
	}
	log.Infoln("Successfully Determined Digest")
	log.Infoln("Calculating Request Signature")
	httpDate := time.Now().UTC().Format(http.TimeFormat)
	host := url.Host
	r.Header.Set("Host", host)
	r.Header.Set("Date", httpDate)
	headers := []string{"(request-target)"}
	pairs := []string{fmt.Sprintf("(request-target): %s %s", strings.ToLower(r.Method), r.URL.Path)}
	for name, value := range r.Header {
		headers = append(headers, strings.ToLower(name))
		pairs = append(pairs, fmt.Sprintf("%s: %s", strings.ToLower(name), value[0]))
	}
	toSign := strings.Join(pairs, "\n")
	hash := sha256.Sum256([]byte(toSign))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create signature: got err=%v", signature)
	}
	encodedSignature := base64.StdEncoding.EncodeToString(signature)
	header := fmt.Sprintf("keyId=%q,headers=%q,signature=%q", keyID, strings.Join(headers, " "), encodedSignature)
	r.Header.Set("Signature", header)
	log.Infoln("Successfully Determined HTTP Request Signature")
	return r, nil
}
