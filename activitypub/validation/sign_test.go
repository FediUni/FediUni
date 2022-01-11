package validation

import (
	"encoding/json"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/go-chi/chi/v5"
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
			privateKey, publicKey, err := actor.NewPKCS1KeyGenerator().GenerateKeyPair()
			if err != nil {
				t.Errorf("Failed to generate private key: got err=%v", err)
			}
			r := chi.NewRouter()
			r.With(Signature).Post("/actor/brandonstark/inbox", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
			r.Get("/actor/brandonstark", func(w http.ResponseWriter, _ *http.Request) {
				marshalledActor, _ := json.Marshal(&actor.Person{
					PublicKey: &actor.PublicKey{
						PublicKeyPem: publicKey,
					},
				})
				w.WriteHeader(http.StatusOK)
				w.Write(marshalledActor)
			})
			server := httptest.NewServer(r)
			serverURL, _ := url.Parse(server.URL)
			defer server.Close()
			request, _ := http.NewRequest("POST", fmt.Sprintf("%s/actor/brandonstark/inbox", serverURL), nil)
			request, err = SignRequest(request, serverURL, fmt.Sprintf("%s/actor/brandonstark", serverURL), privateKey)
			if err != nil {
				t.Errorf("SignRequest(): Unexpected error returned: got err=%v", err)
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
