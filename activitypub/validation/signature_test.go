package validation

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/go-chi/chi/v5"
	"github.com/go-fed/activity/streams"
	"github.com/google/go-cmp/cmp"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
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
				"Signature": []string{`keyId="https://testservice.com/actor#main-key",headers="(request-target) host date",validation="thisisarandomsignature"`},
			},
			wantSignature: map[string]string{
				"keyId":      "https://testservice.com/actor#main-key",
				"headers":    "(request-target) host date",
				"validation": "thisisarandomsignature",
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
			name:           "Test validate request with valid validation (PKCS1)",
			keyGenerator:   actor.NewPKCS1KeyGenerator(),
			wantStatusCode: 200,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testURL, _ := url.Parse("testfediuni.com")
			keyGenerator := actor.NewPKCS1KeyGenerator()
			generator := actor.NewPersonGenerator(testURL, keyGenerator)
			person, err := generator.NewPerson("brandonstark", "8R4ND0N")
			if err != nil {
				t.Errorf("failed to load actor: got err=%v", err)
				return
			}
			r := chi.NewRouter()
			r.With(Signature).Post("/actor/brandonstark/inbox", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
			r.Get("/actor/brandonstark", func(w http.ResponseWriter, _ *http.Request) {
				serializedPerson, _ := streams.Serialize(person)
				marshalledPerson, _ := json.Marshal(serializedPerson)
				w.WriteHeader(http.StatusOK)
				w.Write(marshalledPerson)
			})
			server := httptest.NewServer(r)
			serverURL, _ := url.Parse(server.URL)
			defer server.Close()
			body := []byte("testbody")
			request, _ := http.NewRequest("POST", fmt.Sprintf("%s/actor/brandonstark/inbox", serverURL), bytes.NewBuffer(body))
			block, _ := pem.Decode(keyGenerator.PrivateKeyPEM.Bytes())
			if block == nil {
				t.Errorf("failed to parse PEM block")
			}
			parsedPrivateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			request, err = SignRequestWithDigest(request, serverURL, fmt.Sprintf("%s/actor/brandonstark#main-key", serverURL.String()), parsedPrivateKey, body)
			if err != nil {
				t.Fatalf("failed to create signed request: got err=%v", err)
			}
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
