package activitypub

import (
	"context"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/FediUni/FediUni/activitypub/user"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type TestDatastore struct {
	knownUsers map[string]*actor.Person
}

func NewTestDatastore() *TestDatastore {
	return &TestDatastore{
		knownUsers: map[string]*actor.Person{
			"brandonstark": {
				Context:           nil,
				Type:              "",
				Id:                "https://testfediuni.xyz/actor/brandonstark",
				PreferredUsername: "brandonstark",
				Inbox:             "https://testfediuni.xyz/actor/brandonstark/inbox",
				Outbox:            "https://testfediuni.xyz/actor/brandonstark/outbox",
				Following:         "https://testfediuni.xyz/actor/brandonstark/following",
				Followers:         "https://testfediuni.xyz/actor/brandonstark/followers",
				Liked:             "https://testfediuni.xyz/actor/brandonstark/liked",
				Icon:              "",
				Name:              "Brandon Stark",
				Summary:           "",
				PublicKey:         &actor.PublicKey{},
			},
		},
	}
}

func (d *TestDatastore) GetActor(_ context.Context, username string) (*actor.Person, error) {
	if a := d.knownUsers[username]; a != nil {
		return a, nil
	}
	return nil, fmt.Errorf("unable to find actor with username=%q", username)
}

func (d *TestDatastore) CreateUser(_ context.Context, _ *user.User) error {
	return fmt.Errorf("CreateUser() is Unimplemented")
}

type TestKeyGenerator struct{}

func (g *TestKeyGenerator) GenerateKeyPair() (string, string, error) {
	return "testprivatekey", "testpublickey", nil
}

func (g *TestKeyGenerator) WritePrivateKey(string) error {
	return nil
}

func TestGetActor(t *testing.T) {
	s := NewServer("https://testserver.com", "", NewTestDatastore(), nil)
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
			s := NewServer("https://testserver.com", "", nil, &TestKeyGenerator{})
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
