package validation

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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

func TestAddDigest(t *testing.T) {
	tests := []struct {
		name   string
		digest string
	}{
		{
			name: "Test add Digest header to request with valid body",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := bytes.NewBuffer([]byte("thisisafakebody"))
			hashedBody := sha256.Sum256(body.Bytes())
			test.digest = fmt.Sprintf("sha-256=%s", base64.StdEncoding.EncodeToString(hashedBody[:]))
			request, err := http.NewRequest("POST", "http://testservice.com/actor/brandonstark/inbox", body)
			if err != nil {
				t.Fatalf("failed to create request: got err=%v", err)
			}
			req, err := addDigest(request)
			if err != nil {
				t.Fatalf("failed to add digest: got err=%v", err)
			}
			digest := req.Header.Get("Digest")
			if d := cmp.Diff(test.digest, digest); d != "" {
				t.Errorf("addDigest() returned an unexpected digest: (+got -want) d=%s", d)
			}
		})
	}
}

func TestDigest(t *testing.T) {
	tests := []struct {
		name           string
		wantStatusCode int
	}{
		{
			name:           "Test validate request with valid digest",
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
			r.With(Digest).Post("/actor/brandonstark/inbox", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
			r.Get("/actor/brandonstark", func(w http.ResponseWriter, _ *http.Request) {
				serializedPerson, _ := streams.Serialize(person)
				marshalledPerson, _ := json.Marshal(serializedPerson)
				w.WriteHeader(http.StatusOK)
				w.Write(marshalledPerson)
			})
			server := httptest.NewServer(r)
			serverURL, _ := url.Parse(server.URL)
			defer server.Close()
			body := bytes.NewBuffer([]byte("thisisafakebody"))
			request, err := http.NewRequest("POST", fmt.Sprintf("%s/actor/brandonstark/inbox", serverURL), body)
			if err != nil {
				t.Fatalf("failed to create request: got err=%v", err)
			}
			privateKey, _ := ParsePrivateKeyFromPEMBlock(keyGenerator.PrivateKey.String())
			request, err = SignRequestWithDigest(request, serverURL, fmt.Sprintf("%s/actor/brandonstark", serverURL), privateKey, body.Bytes())
			if err != nil {
				t.Fatalf("SignRequestWithDigest(): Unexpected error returned: got err=%v", err)
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
