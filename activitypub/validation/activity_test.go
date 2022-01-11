package validation

import (
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestActivity(t *testing.T) {
	tests := []struct {
		name           string
		contentType    string
		wantStatusCode int
	}{
		{
			name:           "Test request with unexpected content-type",
			contentType:    "multipart/form-data",
			wantStatusCode: http.StatusBadRequest,
		},

		{
			name:           `Test request with application/ld+json; profile="https://www.w3.org/ns/activitystreams"`,
			contentType:    `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`,
			wantStatusCode: http.StatusOK,
		},

		{
			name:           "Test request with application/ld+json",
			contentType:    "application/ld+json",
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "Test request with application/activity+json",
			contentType:    "application/activity+json",
			wantStatusCode: http.StatusOK,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := chi.NewRouter()
			r.With(Activity).Post("/actor/brandonstark/inbox", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
			server := httptest.NewServer(r)
			serverURL, _ := url.Parse(server.URL)
			defer server.Close()
			request, _ := http.NewRequest("POST", fmt.Sprintf("%s/actor/brandonstark/inbox", serverURL), nil)
			request.Header.Add("Content-Type", test.contentType)
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
