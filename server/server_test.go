package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/FediUni/FediUni/server/activity"
	"github.com/FediUni/FediUni/server/object"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/FediUni/FediUni/server/actor"
	"github.com/FediUni/FediUni/server/client"
	"github.com/FediUni/FediUni/server/user"
	"github.com/go-chi/jwtauth"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TestDatastore struct {
	Datastore
	knownUsers      map[string]*user.User
	knownActors     map[string]actor.Person
	knownActivities map[string]activity.Activity
	privateKeys     map[string]string
}

func NewTestDatastore(url *url.URL, a actor.Person, privateKey *bytes.Buffer) *TestDatastore {
	return &TestDatastore{
		knownUsers: map[string]*user.User{},
		knownActors: map[string]actor.Person{
			"brandonstark": a,
		},
		knownActivities: map[string]activity.Activity{
			"create": generateTestCreate(),
		},
		privateKeys: map[string]string{
			"brandonstark": privateKey.String(),
		},
	}
}

func (d *TestDatastore) GetFollowerStatus(context.Context, string, string) (int, error) {
	return 0, fmt.Errorf("GetFollowerStatus() is unimplemented")
}

func (d *TestDatastore) GetActorByUsername(_ context.Context, username string) (vocab.ActivityStreamsPerson, error) {
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

func (d *TestDatastore) AddActivityToPublicInbox(_ context.Context, _ vocab.Type, _ primitive.ObjectID, b bool) error {
	return fmt.Errorf("AddActivityToPublicInbox() is unimplemented")
}

func (d *TestDatastore) AddActivityToActorInbox(context.Context, vocab.Type, string, *url.URL) error {
	return fmt.Errorf("AddActivityToActorInbox() is unimplemented")
}

func (d *TestDatastore) GetActivityByObjectID(_ context.Context, objectID string, _ string) (activity.Activity, error) {
	activity := d.knownActivities[objectID]
	if activity == nil {
		return nil, fmt.Errorf("failed to load activity with Object ID=%q", objectID)
	}
	return activity, nil
}

func (d *TestDatastore) GetActivityByActivityID(_ context.Context, _ string) (activity.Activity, error) {
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
	user := d.knownUsers[username]
	if user == nil {
		return nil, fmt.Errorf("failed to load user")
	}
	return user, nil
}

func (d *TestDatastore) GetFollowersByUsername(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("GetFollowersByUsername() is unimplemented")
}

func (d *TestDatastore) AddActivityToOutbox(context.Context, vocab.Type, string) error {
	return fmt.Errorf("AddActivityToOutbox() is unimplemented")
}

func (d *TestDatastore) GetFollowingByUsername(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("GetFollowingByUsername() is unimplemented")
}

func (d *TestDatastore) AddObjectsToActorInbox(context.Context, []vocab.Type, string) error {
	return fmt.Errorf("AddObjectsToActorInbox() is unimplemented")
}

func (d *TestDatastore) GetActorInbox(context.Context, string, string, string, bool) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	return nil, fmt.Errorf("GetInboxAsOrderedCollection() is unimplemented")
}

func (d *TestDatastore) GetActorInboxAsOrderedCollection(context.Context, string, bool) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("GetActorInboxAsOrderedCollection() is unimplemented")
}

func (d *TestDatastore) GetActorOutbox(context.Context, string, string, string) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	return nil, fmt.Errorf("GetOutbox() is unimplemented")
}

func (d *TestDatastore) GetActorOutboxAsOrderedCollection(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("GetActorOutboxAsOrderedCollection() is unimplemented")
}

type TestClient struct {
	Client
	knownActors     map[string]actor.Person
	knownActivities map[string]vocab.Type
}

func NewTestClient(a actor.Person) *TestClient {
	return &TestClient{
		knownActors: map[string]actor.Person{
			"@brandonstark@testserver.com": a,
		},
	}
}

func (c *TestClient) FetchRemoteActor(_ context.Context, identifier string) (actor.Actor, error) {
	a := c.knownActors[identifier]
	if a == nil {
		return nil, fmt.Errorf("failed to fetch actor=%q", identifier)
	}
	return a, nil
}

func (c *TestClient) FetchObject(_ context.Context, id *url.URL, _ bool, _ int, _ int) (vocab.Type, error) {
	a := c.knownActivities[id.String()]
	if a == nil {
		return nil, fmt.Errorf("failed to fetch activity ID=%q", id)
	}
	return a, nil
}

type TestKeyGenerator struct{}

func (g *TestKeyGenerator) GenerateKeyPair() (string, string, error) {
	return "testprivatekey", "testpublickey", nil
}

func (g *TestKeyGenerator) WritePrivateKey(string) error {
	return nil
}

func (g *TestKeyGenerator) GetPrivateKeyPEM() ([]byte, error) {
	return []byte("testprivatekey"), nil
}

func TestGetActor(t *testing.T) {
	url, err := url.Parse("https://testserver.com")
	if err != nil {
		t.Fatalf("Failed to parse URL: got err=%v", err)
	}
	keyGenerator := actor.NewRSAKeyGenerator()
	personGenerator := actor.NewPersonGenerator(url, keyGenerator)
	person, _ := personGenerator.NewPerson(context.Background(), "brandonstark", "BR4ND0N")

	tests := []struct {
		name       string
		path       string
		wantRes    actor.Actor
		wantStatus int
		wantErr    bool
	}{
		{
			name:       "Test get non-existent Actor",
			path:       "/api/actor/bendean",
			wantRes:    nil,
			wantStatus: http.StatusNotFound,
			wantErr:    false,
		},
		{
			name:       "Test get existing Actor",
			path:       "/api/actor/brandonstark",
			wantRes:    person,
			wantStatus: http.StatusOK,
			wantErr:    false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			viper.Set("IMAGES_ROOT", "/tmp")
			s, err := New(url, NewTestDatastore(url, person, keyGenerator.PrivateKey), nil, nil, "", "")
			if err != nil {
				t.Fatalf("Failed to create server: got err=%v", err)
			}
			server := httptest.NewServer(s.Router)
			defer server.Close()
			res, err := http.Get(fmt.Sprintf("%s%s", server.URL, test.path))
			if err != nil {
				t.Errorf("getActor() returned an unexpected err: got %v want %v", err, nil)
			}
			defer res.Body.Close()
			body, err := ioutil.ReadAll(res.Body)
			if err != nil {
				t.Errorf("getActor() failed to read from body: got err=%v", err)
			}
			gotStatus := res.StatusCode
			var gotActor map[string]interface{}
			if gotStatus == http.StatusOK {
				if err := json.Unmarshal(body, &gotActor); err != nil {
					t.Errorf("getActor() returned an unexpected result: failed to unmarshal JSON, got err=%v", err)
				}
				gotActor["@context"] = nil
			}
			var wantActor map[string]interface{}
			if test.wantRes != nil {
				wantActor, err = streams.Serialize(test.wantRes)
				if err != nil {
					t.Errorf("getActor() failed to serialize Want Actor: got err=%v", err)
				}
				wantActor["@context"] = nil
			}
			if gotStatus != test.wantStatus {
				t.Errorf("getActor() returned an unexpected status: got %v want %v", gotStatus, test.wantStatus)
			}
			if d := cmp.Diff(wantActor, gotActor); d != "" {
				t.Errorf("getActor() returned an unexpected diff: (+got -want) %s", d)
			}
		})
	}
}

func TestGetAnyActor(t *testing.T) {
	url, err := url.Parse("https://testserver.com")
	if err != nil {
		t.Fatalf("Failed to parse URL: got err=%v", err)
	}
	keyGenerator := actor.NewRSAKeyGenerator()
	personGenerator := actor.NewPersonGenerator(url, keyGenerator)
	person, _ := personGenerator.NewPerson(context.Background(), "brandonstark", "BR4ND0N")

	tests := []struct {
		name       string
		username   string
		domain     string
		wantRes    actor.Actor
		wantStatus int
		wantErr    bool
	}{
		{
			name:       "Test actor does not exist",
			username:   "fakeactor",
			domain:     "testserver.com",
			wantRes:    nil,
			wantStatus: http.StatusNotFound,
			wantErr:    false,
		},
		{
			name:       "Test get actor",
			username:   "brandonstark",
			domain:     "testserver.com",
			wantRes:    person,
			wantStatus: http.StatusOK,
			wantErr:    false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			viper.Set("IMAGES_ROOT", "/tmp")
			s, err := New(url, NewTestDatastore(url, person, keyGenerator.PrivateKey), NewTestClient(person), nil, "", "")
			if err != nil {
				t.Fatalf("Failed to create server: got err=%v", err)
			}
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/actor?identifier=@%s@%s", test.username, test.domain), nil)
			if err != nil {
				t.Errorf("getActor() returned an unexpected err: got %v want %v", err, nil)
			}
			w := httptest.NewRecorder()
			s.getAnyActor(w, req)
			res := w.Result()
			defer res.Body.Close()
			body, err := ioutil.ReadAll(res.Body)
			if err != nil {
				t.Errorf("getActor() failed to read from body: got err=%v", err)
			}
			gotStatus := res.StatusCode
			var gotActor map[string]interface{}
			if gotStatus == http.StatusOK {
				if err := json.Unmarshal(body, &gotActor); err != nil {
					t.Errorf("getActor() returned an unexpected result: failed to unmarshal JSON, got err=%v", err)
				}
				gotActor["@context"] = nil
			}
			var wantActor map[string]interface{}
			if test.wantRes != nil {
				wantActor, err = streams.Serialize(test.wantRes)
				if err != nil {
					t.Errorf("getActor() failed to serialize Want Actor: got err=%v", err)
				}
				wantActor["@context"] = nil
			}
			if gotStatus != test.wantStatus {
				t.Errorf("getActor() returned an unexpected status: got %v want %v", gotStatus, test.wantStatus)
			}
			if d := cmp.Diff(wantActor, gotActor); d != "" {
				t.Errorf("getActor() returned an unexpected diff: (+got -want) %s", d)
			}
		})
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
		{
			name: "Test create bendean",
			params: url.Values{
				"username": []string{"bendean"},
				"password": []string{"fakepassword"},
			},
			wantStatus: http.StatusOK,
		},
	}
	url, err := url.Parse("https://testserver.com")
	if err != nil {
		t.Fatalf("Failed to parse URL: got err=%v", err)
	}
	for _, test := range tests {
		redis := miniredis.RunT(t)
		buffer := &bytes.Buffer{}
		w := multipart.NewWriter(buffer)
		for fieldName, property := range test.params {
			err := w.WriteField(fieldName, property[0])
			if err != nil {
				t.Fatalf("CreateUser() failed to create multipart form: got err=%v", err)
			}
		}
		w.Close()
		t.Run(test.name, func(t *testing.T) {
			s, _ := New(url, NewTestDatastore(url, nil, nil), nil, &TestKeyGenerator{}, "fakesecret", redis.Addr())
			server := httptest.NewServer(s.Router)
			defer server.Close()
			registrationURL := fmt.Sprintf("%s/api/register", server.URL)
			resp, err := http.Post(registrationURL, w.FormDataContentType(), buffer)
			if err != nil {
				t.Errorf("%s: returned an unexpected err: got err=%v", registrationURL, err)
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
	url, err := url.Parse("https://testfediuni.xyz")
	if err != nil {
		t.Fatalf("Failed to parse URL: got err=%v", err)
	}
	keyGenerator := actor.NewRSAKeyGenerator()
	personGenerator := actor.NewPersonGenerator(url, keyGenerator)
	person, _ := personGenerator.NewPerson(context.Background(), "brandonstark", "BR4ND0N")

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
	if err != nil {
		t.Fatalf("Failed to parse URL: got err=%v", err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, _ := New(url, NewTestDatastore(url, person, keyGenerator.PrivateKey), nil, nil, "", "")
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
				t.Fatalf("%s: Failed to read response returned: got err=%v", webfingerURL, err)
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
	url, err := url.Parse("https://testserver.com")
	if err != nil {
		t.Fatalf("Failed to parse URL: got err=%v", err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, _ := New(url, NewTestDatastore(url, nil, nil), nil, nil, "", "")
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
		wantErr  bool
	}{
		{
			name:     "Test valid username and password",
			username: "testuser",
			password: "testpassword",
			wantErr:  false,
		},
		{
			name:     "Test invalid password",
			username: "testuser",
			password: "badpassword",
			wantErr:  true,
		},

		{
			name:     "Test invalid username",
			username: "fakeuser",
			password: "testpassword",
			wantErr:  true,
		},
	}
	url, err := url.Parse("https://testserver.com")
	if err != nil {
		t.Fatalf("Failed to parse URL: got err=%v", err)
	}
	u := &user.User{
		Username: "testuser",
	}
	hashedPassword, err := user.HashPassword("testpassword")
	if err != nil {
		t.Fatalf("failed to hash password: got err=%v", err)
	}
	u.Password = string(hashedPassword)
	for _, test := range tests {
		datastore := NewTestDatastore(url, nil, nil)
		if err := datastore.CreateUser(context.Background(), u); err != nil {
			t.Fatalf("failed to create user in datastore: got err=%v", err)
		}
		t.Run(test.name, func(t *testing.T) {
			secret := "thisisatestsecret"
			s, _ := New(url, datastore, nil, nil, secret, "")
			server := httptest.NewServer(s.Router)
			defer server.Close()
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
			if res.StatusCode != http.StatusOK && test.wantErr {
				return
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
			if err != nil && !test.wantErr {
				t.Errorf("failed to verify JWT token: got err=%v", err)
			}
		})
	}
}

func TestGetActivity(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantCode     int
		wantActivity vocab.Type
	}{
		{
			name:         "Test GET Activity that does not exist",
			path:         "/api/activity/fakeactivity",
			wantCode:     http.StatusNotFound,
			wantActivity: nil,
		},
		{
			name:         "Test GET Create Activity that exists",
			path:         "/api/activity/create",
			wantCode:     http.StatusOK,
			wantActivity: generateTestCreate(),
		},
	}
	serverURL, err := url.Parse("https://testserver.com")
	if err != nil {
		t.Fatalf("Failed to parse URL: got err=%v", err)
	}
	for _, test := range tests {
		s, _ := New(serverURL, NewTestDatastore(nil, nil, nil), nil, nil, "", "")
		server := httptest.NewServer(s.Router)
		defer server.Close()
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s%s", server.URL, test.path), nil)
		if err != nil {
			t.Fatalf("Failed to create Activity request: got err=%v", err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("getActivity() returned an unexpected error: got err=%v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != test.wantCode {
			t.Errorf("getActivity() returned an unexpected HTTP StatusCode: got=%d, want=%d", res.StatusCode, test.wantCode)
		}
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Errorf("getActivity() failed to return a response body: got err=%v", err)
		}
		if res.StatusCode != http.StatusOK {
			return
		}
		var gotActivity vocab.Type
		var m map[string]interface{}
		if err := json.Unmarshal(body, &m); err != nil {
			t.Errorf("getActivity() failed to return a JSON body: got err=%v", err)
		}
		gotActivity, err = streams.ToType(context.Background(), m)
		if err != nil {
			t.Errorf("getActivity() failed to return an activity: got err=%v", err)
		}
		if d := cmp.Diff(test.wantActivity, gotActivity); d != "" {
			t.Errorf("getActivity() returned an unexpected diff: (+got -want) %s", d)
		}
	}
}

func TestGetAnyActivity(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantCode     int
		wantActivity vocab.Type
	}{
		{
			name:         "Test GET Activity that does not exist",
			path:         "/api/activity/fakeactivity",
			wantCode:     http.StatusNotFound,
			wantActivity: nil,
		},
		{
			name:         "Test GET Create Activity that exists",
			path:         "/api/activity/create",
			wantCode:     http.StatusOK,
			wantActivity: generateTestCreate(),
		},
	}
	serverURL, err := url.Parse("https://testserver.com")
	if err != nil {
		t.Fatalf("Failed to parse URL: got err=%v", err)
	}
	for _, test := range tests {
		s, _ := New(serverURL, NewTestDatastore(nil, nil, nil), NewTestClient(nil), nil, "", "")
		// Use a different router to remove authentication middleware.
		r := chi.NewRouter()
		r.Get("/api/activity", s.getAnyActivity)
		server := httptest.NewServer(r)
		defer server.Close()
		activityID := fmt.Sprintf("%s%s", server.URL, test.path)
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/activity?id=%s", server.URL, activityID), nil)
		if err != nil {
			t.Fatalf("Failed to create Activity request: got err=%v", err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("getActivity() returned an unexpected error: got err=%v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != test.wantCode {
			t.Errorf("getActivity() returned an unexpected HTTP StatusCode: got=%d, want=%d", res.StatusCode, test.wantCode)
		}
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Errorf("getActivity() failed to return a response body: got err=%v", err)
		}
		if res.StatusCode != http.StatusOK {
			return
		}
		var gotActivity vocab.Type
		var m map[string]interface{}
		if err := json.Unmarshal(body, &m); err != nil {
			t.Errorf("getActivity() failed to return a JSON body: got err=%v", err)
		}
		gotActivity, err = streams.ToType(context.Background(), m)
		if err != nil {
			t.Errorf("getActivity() failed to return an activity: got err=%v", err)
		}
		if d := cmp.Diff(test.wantActivity, gotActivity); d != "" {
			t.Errorf("getActivity() returned an unexpected diff: (+got -want) %s", d)
		}
	}
}

func TestGetActivityObject(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantCode   int
		wantObject vocab.Type
	}{
		{
			name:     "Test GET object that does not exist",
			path:     "/api/activity/fakeactivity/object",
			wantCode: http.StatusNotFound,
		},
		{
			name:       "Test GET Note in Create Activity",
			path:       "/api/activity/create/object",
			wantCode:   http.StatusOK,
			wantObject: generateTestNote(),
		},
	}
	serverURL, err := url.Parse("https://testserver.com")
	if err != nil {
		t.Fatalf("Failed to parse URL: got err=%v", err)
	}
	for _, test := range tests {
		s, _ := New(serverURL, NewTestDatastore(nil, nil, nil), nil, nil, "", "")
		server := httptest.NewServer(s.Router)
		defer server.Close()
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s%s", server.URL, test.path), nil)
		if err != nil {
			t.Fatalf("Failed to create Activity request: got err=%v", err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("getActivity() returned an unexpected error: got err=%v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != test.wantCode {
			t.Errorf("getActivity() returned an unexpected HTTP StatusCode: got=%d, want=%d", res.StatusCode, test.wantCode)
		}
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Errorf("getActivity() failed to return a response body: got err=%v", err)
		}
		if res.StatusCode != http.StatusOK {
			return
		}
		var gotObject vocab.Type
		var m map[string]interface{}
		if err := json.Unmarshal(body, &m); err != nil {
			t.Errorf("getActivity() failed to return a JSON body: got err=%v", err)
		}
		gotObject, err = streams.ToType(context.Background(), m)
		if err != nil {
			t.Errorf("getActivity() failed to return an activity: got err=%v", err)
		}
		if d := cmp.Diff(test.wantObject, gotObject); d != "" {
			t.Errorf("getActivity() returned an unexpected diff: (+got -want) %s", d)
		}
	}
}

func generateTestPerson() vocab.ActivityStreamsPerson {
	p := streams.NewActivityStreamsPerson()
	id, _ := url.Parse("http://testserver.com/actor/brandonstark")
	idProperty := streams.NewJSONLDIdProperty()
	idProperty.Set(id)
	p.SetJSONLDId(idProperty)
	outboxProperty := streams.NewActivityStreamsOutboxProperty()
	outboxID, _ := url.Parse("http://testserver.com/actor/brandonstark/outbox")
	outboxProperty.SetIRI(outboxID)
	p.SetActivityStreamsOutbox(outboxProperty)
	return p
}

func generateTestCreate() vocab.ActivityStreamsCreate {
	create, _ := object.WrapInCreate(context.Background(), generateTestNote(), generateTestPerson())
	return create
}

func generateTestNote() vocab.ActivityStreamsNote {
	note := streams.NewActivityStreamsNote()
	id, _ := url.Parse("http://testserver.com/actor/brandonstark/outbox/note")
	idProperty := streams.NewJSONLDIdProperty()
	idProperty.Set(id)
	return note
}
