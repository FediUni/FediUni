package validation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/go-chi/chi/v5"
	"github.com/go-fed/activity/streams"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSignRequest(t *testing.T) {
	tests := []struct {
		name           string
		wantStatusCode int
	}{
		{
			name:           "Test validate request with valid signature",
			wantStatusCode: 200,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testURL, _ := url.Parse("testfediuni.com")
			keyGenerator := actor.NewRSAKeyGenerator()
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
			body := bytes.NewBuffer([]byte("thisisafakebody"))
			request, err := http.NewRequest("POST", fmt.Sprintf("%s/actor/brandonstark/inbox", serverURL), body)
			if err != nil {
				t.Fatalf("failed to create request: got err=%v", err)
			}
			privateKey, _ := ParsePrivateKeyFromPEMBlock(keyGenerator.PrivateKey.String())
			request, err = SignRequestWithDigest(request, serverURL, fmt.Sprintf("%s/actor/brandonstark", serverURL), privateKey, body.Bytes())
			if err != nil {
				t.Errorf("SignRequestWithDigest(): Unexpected error returned: got err=%v", err)
			}
			res, err := server.Client().Do(request)
			if err != nil {
				t.Fatalf("Unexpected error returned: got err=%v", err)
			}
			defer res.Body.Close()
			if res.StatusCode != test.wantStatusCode {
				t.Errorf("Unexpected response returned: got=%d, want=%d", res.StatusCode, test.wantStatusCode)
			}
		})
	}
}
