package activitypub

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/FediUni/FediUni/activitypub/user"
	"github.com/go-fed/activity/streams/vocab"
	"github.com/google/go-cmp/cmp"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type TestDatastore struct {
	knownUsers  map[string]actor.Person
	privateKeys map[string]string
}

func NewTestDatastore(rawURL string) *TestDatastore {
	parsedURL, _ := url.Parse(rawURL)
	keyGenerator := actor.NewPKCS1KeyGenerator()
	personGenerator := actor.NewPersonGenerator(parsedURL, keyGenerator)
	person, _ := personGenerator.NewPerson("brandonstark", "BR4ND0N")
	return &TestDatastore{
		knownUsers: map[string]actor.Person{
			"brandonstark": person,
		},
		privateKeys: map[string]string{
			"brandonstark": keyGenerator.PrivateKey.String(),
		},
	}
}

func (d *TestDatastore) GetActorByUsername(_ context.Context, username string) (actor.Person, error) {
	if a := d.knownUsers[username]; a != nil {
		return a, nil
	}
	return nil, fmt.Errorf("unable to find actor with username=%q", username)
}

func (d *TestDatastore) GetActorByActorID(_ context.Context, _ string) (actor.Person, error) {
	return nil, fmt.Errorf("GetActorByActorID() is unimplemented")
}

func (d *TestDatastore) CreateUser(_ context.Context, _ *user.User) error {
	return fmt.Errorf("CreateUser() is Unimplemented")
}

func (d *TestDatastore) AddActivityToSharedInbox(_ context.Context, _ vocab.Type, _ string) error {
	return fmt.Errorf("AddActivityToSharedInbox() is unimplemented")
}

func (d *TestDatastore) GetActivity(_ context.Context, _ string, _ string) (vocab.Type, error) {
	return nil, fmt.Errorf("GetActivity() is unimplemented")
}

func (d *TestDatastore) AddFollowerToActor(context.Context, string, string) error {
	return fmt.Errorf("AddFollowerToActor() is unimplemented")
}

func (d *TestDatastore) RemoveFollowerFromActor(ctx context.Context, actorID, followerID string) error {
	return fmt.Errorf("RemoveFollowerFromActor() is unimplemented")
}

type TestKeyGenerator struct{}

func (g *TestKeyGenerator) GenerateKeyPair() (string, string, error) {
	return "testprivatekey", "testpublickey", nil
}

func (g *TestKeyGenerator) WritePrivateKey(string) error {
	return nil
}

func TestGetActor(t *testing.T) {
	s, _ := NewServer("https://testserver.com", "", NewTestDatastore("https://testserver.com"), nil)
	server := httptest.NewServer(s.Router)
	defer server.Close()
	resp, err := http.Get(fmt.Sprintf("%s/actor/bendean", server.URL))
	if err != nil {
		t.Errorf("getActor(): returned an unexpected err: got %v want %v", err, nil)
	}
	defer resp.Body.Close()
	gotStatus := resp.StatusCode
	wantStatus := http.StatusNotFound
	if gotStatus != wantStatus {
		t.Errorf("getActor(): returned an unexpected status: got %v want %v", gotStatus, wantStatus)
	}
}

func TestCreateUser(t *testing.T) {
	tests := []struct {
		name       string
		params     url.Values
		wantStatus int
	}{
		{
			name: "Test no username provided",
			params: url.Values{
				"username": []string{""},
				"password": []string{"testpassword"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Test no password provided",
			params: url.Values{
				"username": []string{"testuser"},
				"password": []string{""},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Test no username or password provided",
			params: url.Values{
				"username": []string{""},
				"password": []string{""},
			},
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, _ := NewServer("https://testserver.com", "", nil, &TestKeyGenerator{})
			server := httptest.NewServer(s.Router)
			defer server.Close()
			registrationURL := fmt.Sprintf("%s/register", server.URL)
			resp, err := http.PostForm(registrationURL, test.params)
			if err != nil {
				t.Errorf("%s: returned an unexpected err: got=%v want=%v", registrationURL, err, nil)
			}
			defer resp.Body.Close()
			gotStatus := resp.StatusCode
			if gotStatus != test.wantStatus {
				t.Errorf("%s: returned an unexpected status: got %v want %v", registrationURL, gotStatus, test.wantStatus)
			}
		})
	}
}

func TestWebfingerKnownAccount(t *testing.T) {
	tests := []struct {
		name         string
		resource     string
		wantResponse *WebfingerResponse
	}{
		{
			name:     "Test load account belonging to instance",
			resource: "acct:brandonstark@testfediuni.xyz",
			wantResponse: &WebfingerResponse{
				Subject: "acct:brandonstark@testfediuni.xyz",
				Links: []WebfingerLink{
					{
						Rel:  "self",
						Type: "application/activity+json",
						Href: "https://testfediuni.xyz/actor/brandonstark",
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, _ := NewServer("https://testfediuni.xyz", "", NewTestDatastore("https://testfediuni.xyz"), nil)
			server := httptest.NewServer(s.Router)
			defer server.Close()
			webfingerURL := fmt.Sprintf("%s/.well-known/webfinger", server.URL)
			resp, err := http.Get(fmt.Sprintf("%s?resource=%s", webfingerURL, test.resource))
			if err != nil {
				t.Errorf("%s: returned an unexpected err: got=%v want=%v", webfingerURL, err, nil)
			}
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("%s: Failed to read response returned: got err=%v", webfingerURL, err)
			}
			var gotResponse *WebfingerResponse
			if err := json.Unmarshal(body, &gotResponse); err != nil {
				t.Errorf("%s: Failed to unmarshal response: got err=%v", webfingerURL, err)
			}
			if d := cmp.Diff(test.wantResponse, gotResponse); d != "" {
				t.Errorf("%s: returned an unexpected diff (-want +got): %s", webfingerURL, d)
			}
		})
	}
}

func TestWebfinger(t *testing.T) {
	tests := []struct {
		name          string
		resource      string
		wantErrorCode int
	}{
		{
			name:          "Test load account not belonging to instance",
			resource:      "acct:fakeacccount@testfediuni.xyz",
			wantErrorCode: http.StatusNotFound,
		},
		{
			name:          "Test bad resource syntax (missing \"acct:\")",
			resource:      "fakeacccount@testfediuni.xyz",
			wantErrorCode: http.StatusBadRequest,
		},
		{
			name:          "Test bad resource syntax (resource parameter empty)",
			resource:      "",
			wantErrorCode: http.StatusBadRequest,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, _ := NewServer("https://testfediuni.xyz", "", NewTestDatastore("https://testserver.com"), nil)
			server := httptest.NewServer(s.Router)
			defer server.Close()
			webfingerURL := fmt.Sprintf("%s/.well-known/webfinger", server.URL)
			resp, err := http.Get(fmt.Sprintf("%s?resource=%s", webfingerURL, test.resource))
			if err != nil {
				t.Errorf("%s: returned an unexpected err: got=%v want=%v", webfingerURL, err, nil)
			}
			defer resp.Body.Close()
			gotStatus := resp.StatusCode
			if gotStatus != test.wantErrorCode {
				t.Errorf("%s: returned an unexpected status: got %v want %v", webfingerURL, gotStatus, test.wantErrorCode)
			}
		})
	}
}
