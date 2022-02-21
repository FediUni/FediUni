package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/jwtauth"

	"github.com/FediUni/FediUni/server/actor"
	"github.com/FediUni/FediUni/server/client"
	"github.com/FediUni/FediUni/server/user"
	"github.com/go-fed/activity/streams/vocab"
	"github.com/google/go-cmp/cmp"
)

type TestDatastore struct {
	knownUsers  map[string]*user.User
	knownActors map[string]actor.Person
	privateKeys map[string]string
}

func NewTestDatastore(rawURL string) *TestDatastore {
	parsedURL, _ := url.Parse(rawURL)
	keyGenerator := actor.NewRSAKeyGenerator()
	personGenerator := actor.NewPersonGenerator(parsedURL, keyGenerator)
	person, _ := personGenerator.NewPerson("brandonstark", "BR4ND0N")
	return &TestDatastore{
		knownUsers: map[string]*user.User{},
		knownActors: map[string]actor.Person{
			"brandonstark": person,
		},
		privateKeys: map[string]string{
			"brandonstark": keyGenerator.PrivateKey.String(),
		},
	}
}

func (d *TestDatastore) GetFollowerStatus(context.Context, string, string) (int, error) {
	return 0, fmt.Errorf("GetFollowerStatus() is unimplemented")
}

func (d *TestDatastore) GetActorByUsername(_ context.Context, username string) (actor.Person, error) {
	if a := d.knownActors[username]; a != nil {
		return a, nil
	}
	return nil, fmt.Errorf("unable to find actor with username=%q", username)
}

func (d *TestDatastore) GetActorByActorID(_ context.Context, _ string) (actor.Person, error) {
	return nil, fmt.Errorf("GetActorByActorID() is unimplemented")
}

func (d *TestDatastore) CreateUser(_ context.Context, user *user.User) error {
	d.knownUsers[user.Username] = user
	return nil
}

func (d *TestDatastore) AddActivityToSharedInbox(_ context.Context, _ vocab.Type, _ string) error {
	return fmt.Errorf("AddActivityToSharedInbox() is unimplemented")
}

func (d *TestDatastore) AddActivityToActorInbox(context.Context, vocab.Type, string) error {
	return fmt.Errorf("AddActivityToActorInbox() is unimplemented")
}

func (d *TestDatastore) GetActivityByObjectID(_ context.Context, _ string, _ string) (vocab.Type, error) {
	return nil, fmt.Errorf("GetActivityByObjectID() is unimplemented")
}

func (d *TestDatastore) GetActivityByActivityID(_ context.Context, _ string) (vocab.Type, error) {
	return nil, fmt.Errorf("GetActivityByObjectID() is unimplemented")
}

func (d *TestDatastore) AddFollowerToActor(context.Context, string, string) error {
	return fmt.Errorf("AddFollowerToActor() is unimplemented")
}

func (d *TestDatastore) AddActorToFollows(context.Context, string, string) error {
	return fmt.Errorf("AddActorToFollows() is unimplemented")
}

func (d *TestDatastore) RemoveFollowerFromActor(ctx context.Context, actorID, followerID string) error {
	return fmt.Errorf("RemoveFollowerFromActor() is unimplemented")
}

func (d *TestDatastore) GetUserByUsername(_ context.Context, username string) (*user.User, error) {
	return d.knownUsers[username], nil
}

func (d *TestDatastore) GetFollowersByUsername(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("GetFollowersByUsername() is unimplemented")
}

func (d *TestDatastore) GetFollowingByUsername(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("GetFollowingByUsername() is unimplemented")
}

func (d *TestDatastore) AddObjectsToActorInbox(context.Context, []vocab.Type, string) error {
	return fmt.Errorf("AddObjectsToActorInbox() is unimplemented")
}

func (d *TestDatastore) GetActorInbox(context.Context, string, string, string, string) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	return nil, fmt.Errorf("GetActorInbox() is unimplemented")
}

func (d *TestDatastore) GetActorInboxAsOrderedCollection(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("GetActorInboxAsOrderedCollection() is unimplemented")
}

func (d *TestDatastore) GetActorOutbox(context.Context, string, string, string, string) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	return nil, fmt.Errorf("GetActorOutbox() is unimplemented")
}

func (d *TestDatastore) GetActorOutboxAsOrderedCollection(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("GetActorOutboxAsOrderedCollection() is unimplemented")
}

type TestKeyGenerator struct{}

func (g *TestKeyGenerator) GenerateKeyPair() (string, string, error) {
	return "testprivatekey", "testpublickey", nil
}

func (g *TestKeyGenerator) WritePrivateKey(string) error {
	return nil
}

func (g *TestKeyGenerator) GetPrivateKeyPEM() ([]byte, error) {
	return nil, fmt.Errorf("GetPrivateKeyPEM() is unimplemented")
}

func TestGetActor(t *testing.T) {
	s, _ := New("https://testserver.com", NewTestDatastore("https://testserver.com"), nil, "")
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
			s, _ := New("https://testserver.com", nil, &TestKeyGenerator{}, "")
			server := httptest.NewServer(s.Router)
			defer server.Close()
			registrationURL := fmt.Sprintf("%s/api/register", server.URL)
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
		wantResponse *client.WebfingerResponse
	}{
		{
			name:     "Test load account belonging to instance",
			resource: "acct:brandonstark@testfediuni.xyz",
			wantResponse: &client.WebfingerResponse{
				Subject: "acct:brandonstark@testfediuni.xyz",
				Links: []client.WebfingerLink{
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
			s, _ := New("https://testfediuni.xyz", NewTestDatastore("https://testfediuni.xyz"), nil, "")
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
			var gotResponse *client.WebfingerResponse
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
			s, _ := New("https://testfediuni.xyz", NewTestDatastore("https://testserver.com"), nil, "")
			server := httptest.NewServer(s.Router)
			defer server.Close()
			webfingerURL := fmt.Sprintf("%s/.well-known/webfinger", server.URL)
			res, err := http.Get(fmt.Sprintf("%s?resource=%s", webfingerURL, test.resource))
			if err != nil {
				t.Errorf("%s: returned an unexpected err: got=%v want=%v", webfingerURL, err, nil)
			}
			defer res.Body.Close()
			gotStatus := res.StatusCode
			if gotStatus != test.wantErrorCode {
				t.Errorf("%s: returned an unexpected status: got %v want %v", webfingerURL, gotStatus, test.wantErrorCode)
			}
		})
	}
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		params   url.Values
	}{
		{
			name:     "Test valid username and password",
			username: "testuser",
			password: "testpassword",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			datastore := NewTestDatastore("https://testserver.com")
			secret := "thisisatestsecret"
			s, _ := New("https://testfediuni.xyz", datastore, nil, secret)
			server := httptest.NewServer(s.Router)
			defer server.Close()
			u := &user.User{
				Username: test.username,
			}
			hashedPassword, err := user.HashPassword(test.password)
			if err != nil {
				t.Fatalf("failed to hash password: got err=%v", err)
			}
			u.Password = string(hashedPassword)
			if err := datastore.CreateUser(context.Background(), u); err != nil {
				t.Fatalf("failed to create user in datastore: got err=%v", err)
			}
			loginURL := fmt.Sprintf("%s/api/login", server.URL)
			buf := &bytes.Buffer{}
			writer := multipart.NewWriter(buf)
			writer.WriteField("username", test.username)
			writer.WriteField("password", test.password)
			writer.Close()
			req, err := http.NewRequest(http.MethodPost, loginURL, buf)
			if err != nil {
				t.Fatalf("failed to create login form: got err=%v", err)
			}
			req.Header.Set("Content-Type", writer.FormDataContentType())
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("failed to POST login form: got err=%v", err)
			}
			defer res.Body.Close()
			body, err := ioutil.ReadAll(res.Body)
			if err != nil {
				t.Errorf("failed to read from response body: got err=%v", err)
			}
			var m map[string]interface{}
			if err := json.Unmarshal(body, &m); err != nil {
				t.Errorf("failed to unmarshal JSON from response body: got err=%v", err)
			}
			var token string
			switch m["jwt"].(type) {
			case string:
				token = m["jwt"].(string)
			default:
				t.Errorf("failed to receive token string in JSON: got %v", token)
			}
			tokenAuth = jwtauth.New("HS256", []byte(secret), nil)
			_, err = jwtauth.VerifyToken(tokenAuth, token)
			if err != nil {
				t.Errorf("failed to verify JWT token: got err=%v", err)
			}
		})
	}
}
