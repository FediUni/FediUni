package validation

import (
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
			request, _ := http.NewRequest("POST", fmt.Sprintf("%s/actor/brandonstark/inbox", serverURL), nil)
			request, err = SignRequest(request, serverURL, fmt.Sprintf("%s/actor/brandonstark", serverURL), keyGenerator.PrivateKey.String())
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
