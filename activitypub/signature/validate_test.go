package signature

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/go-chi/chi/v5"
	"github.com/google/go-cmp/cmp"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestProcessSignatureHeader(t *testing.T) {
	tests := []struct {
		name          string
		header        http.Header
		wantSignature map[string]string
		wantErr       bool
	}{
		{
			name: "Test typical Signature header",
			header: http.Header{
				"Signature": []string{`keyId="https://testservice.com/actor#main-key",headers="(request-target) host date",signature="thisisarandomsignature"`},
			},
			wantSignature: map[string]string{
				"keyId":     "https://testservice.com/actor#main-key",
				"headers":   "(request-target) host date",
				"signature": "thisisarandomsignature",
			},
		},
		{
			name:    "Test empty Signature header",
			header:  http.Header{},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotSignature, err := processSignatureHeader(test.header)
			if err != nil && !test.wantErr {
				t.Errorf("processSignatureHeader(): returned an unexpected error: got err=%v", err)
			}
			if err == nil && test.wantErr {
				t.Errorf("processSignatureHeader(): returned an unexpected error: got=%v, want=%v", err, nil)
			}
			if d := cmp.Diff(test.wantSignature, gotSignature); d != "" {
				t.Errorf("processSignatureHeader(): returned an unexpected diff (-want +got): %s", d)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name           string
		keyGenerator   *actor.PKCS1KeyGenerator
		wantStatusCode int
	}{
		{
			name:           "Test validate request with valid signature (PKCS1)",
			keyGenerator:   actor.NewPKCS1KeyGenerator(),
			wantStatusCode: 200,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			privateKey, publicKey, err := test.keyGenerator.GenerateKeyPair()
			if err != nil {
				t.Errorf("Failed to generate private key: got err=%v", err)
			}
			r := chi.NewRouter()
			r.With(Validate).Post("/actor/brandonstark/inbox", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
			r.Get("/actor/brandonstark", func(w http.ResponseWriter, _ *http.Request) {
				marshalledActor, _ := json.Marshal(&actor.Person{
					PublicKey: &actor.PublicKey{
						PublicKeyPem: publicKey,
					},
				})
				w.Write(marshalledActor)
			})
			server := httptest.NewServer(r)
			serverURL, _ := url.Parse(server.URL)
			defer server.Close()
			request, _ := http.NewRequest("POST", fmt.Sprintf("%s/actor/brandonstark/inbox", serverURL), nil)
			httpDate := time.Now().UTC().Format(http.TimeFormat)
			request.Header.Set("Host", serverURL.Host)
			request.Header.Set("Date", httpDate)
			headers := []string{"(request-target)"}
			pairs := []string{"(request-target): post /actor/brandonstark/inbox"}
			for name, value := range request.Header {
				headers = append(headers, name)
				pairs = append(pairs, fmt.Sprintf("%s: %s", name, value[0]))
			}
			toSign := strings.Join(pairs, "\n")
			hash := sha256.Sum256([]byte(toSign))
			block, _ := pem.Decode([]byte(privateKey))
			if block == nil {
				t.Errorf("failed to parse PEM block")
			}
			parsedPrivateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			signature, err := rsa.SignPKCS1v15(rand.Reader, parsedPrivateKey, crypto.SHA256, hash[:])
			if err != nil {
				t.Errorf("failed to create signature: got err=%v", signature)
			}
			header := fmt.Sprintf("keyId=%q,headers=%q,signature=%q", fmt.Sprintf("%s/actor/brandonstark#main-key", serverURL.String()), strings.Join(headers, " "), base64.StdEncoding.EncodeToString(signature))
			request.Header.Set("Signature", header)
			res, err := server.Client().Do(request)
			if err != nil {
				t.Errorf("Unexpected error returned: got err=%v", err)
			}
			if res.StatusCode != test.wantStatusCode {
				t.Errorf("Unexpected response returned: got=%d, want=%d", res.StatusCode, test.wantStatusCode)
			}
		})
	}
}
