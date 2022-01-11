package validation

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func SignRequest(r *http.Request, url *url.URL, keyID string, privateKey string) (*http.Request, error) {
	if keyID == "" {
		return nil, fmt.Errorf("keyID must be specified, got=%q", keyID)
	}
	if url.Host == "" {
		return nil, fmt.Errorf("URL host must be specifed, got=%q", url.String())
	}
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
	fmt.Println(toSign)
	hash := sha256.Sum256([]byte(toSign))
	block, _ := pem.Decode([]byte(privateKey))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}
	parsedPrivateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	signature, err := rsa.SignPKCS1v15(rand.Reader, parsedPrivateKey, crypto.SHA256, hash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create signature: got err=%v", signature)
	}
	encodedSignature := base64.StdEncoding.EncodeToString(signature)
	header := fmt.Sprintf("keyId=%q,headers=%q,signature=%q", keyID, strings.Join(headers, " "), encodedSignature)
	r.Header.Set("Signature", header)
	return r, nil
}
