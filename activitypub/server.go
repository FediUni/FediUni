package activitypub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/activity"
	"github.com/FediUni/FediUni/activitypub/client"
	"github.com/FediUni/FediUni/activitypub/follower"
	"github.com/FediUni/FediUni/activitypub/undo"
	"github.com/FediUni/FediUni/activitypub/validation"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/FediUni/FediUni/activitypub/user"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	log "github.com/golang/glog"
)

type Datastore interface {
	GetActorByUsername(context.Context, string) (actor.Person, error)
	GetActivity(context.Context, string, string) (vocab.Type, error)
	CreateUser(context.Context, *user.User) error
	AddActivityToSharedInbox(context.Context, vocab.Type, string) error
	AddFollowerToActor(context.Context, string, string) error
	RemoveFollowerFromActor(context.Context, string, string) error
	GetActorByActorID(context.Context, string) (actor.Person, error)
}

type Server struct {
	URL          *url.URL
	Keys         string
	Router       *chi.Mux
	Datastore    Datastore
	KeyGenerator actor.KeyGenerator
	Client       *client.Client
}

type WebfingerResponse struct {
	Subject string          `json:"subject"`
	Links   []WebfingerLink `json:"links"`
}

type WebfingerLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type"`
	Href string `json:"href"`
}

func NewServer(instanceURL, keys string, datastore Datastore, keyGenerator actor.KeyGenerator) (*Server, error) {
	url, err := url.Parse(instanceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse instanceURL=%q: got err=%v", instanceURL, err)
	}
	s := &Server{
		URL:          url,
		Keys:         keys,
		Datastore:    datastore,
		KeyGenerator: keyGenerator,
	}
	s.Router = chi.NewRouter()

	s.Router.Use(middleware.Logger)
	s.Router.Use(middleware.Timeout(60 * time.Second))
	s.Router.Use(httprate.LimitAll(100, time.Minute*1))

	s.Router.Get("/", s.homepage)
	s.Router.Get("/.well-known/webfinger", s.webfinger)
	s.Router.Get("/actor/{username}", s.getActor)
	s.Router.Get("/actor/{username}/inbox", s.getActorInbox)
	s.Router.Get("/activity/{activityID}", s.getActivity)
	s.Router.With(validation.Signature).Post("/actor/{username}/inbox", s.receiveToActorInbox)
	s.Router.Get("/actor/{username}/outbox", s.getActorOutbox)
	s.Router.Post("/register", s.createUser)
	return s, nil
}

func (s *Server) homepage(w http.ResponseWriter, _ *http.Request) {
	homeTemplate := template.New("Home")
	homeTemplate, err := homeTemplate.Parse(`<html>
		<head>
			<title>FediUni</title>
		</head>
		<body>
			<p>This website is a WIP instance of the FediUni application. The source code for this application can be found <a href="https://github.com/FediUni/FediUni">here</a>.</p>
		</body>
	</html>`)
	if err != nil {
		log.Errorf("failed to parse home page template: got err=%v", err)
		return
	}
	homeTemplate.Execute(w, "Home")
}

func (s *Server) getActor(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if username == "" {
		http.Error(w, "username is unspecified", http.StatusBadRequest)
		return
	}
	person, err := s.Datastore.GetActorByUsername(r.Context(), username)
	if err != nil {
		log.Errorf("failed to get actor with ID=%q: got err=%v", username, err)
		http.Error(w, "failed to load actor", http.StatusNotFound)
		return
	}
	serializedPerson, err := streams.Serialize(person)
	if err != nil {
		log.Errorf("failed to serialize actor with ID=%q: got err=%v", username, err)
		http.Error(w, "failed to load actor", http.StatusInternalServerError)
	}
	m, err := json.Marshal(serializedPerson)
	if err != nil {
		log.Errorf("failed to marshal actor with ID=%q: got err=%v", username, err)
		http.Error(w, "failed to load actor", http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

func (s *Server) getActivity(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "activityID")
	if activityID == "" {
		http.Error(w, "activityID is unspecified", http.StatusBadRequest)
	}
	activity, err := s.Datastore.GetActivity(r.Context(), activityID, s.URL.String())
	if err != nil {
		log.Errorf("failed to get activity with ID=%q: got err=%v", activityID, err)
		http.Error(w, "failed to load activity", http.StatusNotFound)
		return
	}
	marshalledActivity, err := json.Marshal(activity)
	if err != nil {
		log.Errorf("failed to get activity with ID=%q: got err=%v", activityID, err)
		http.Error(w, "failed to load activity", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(marshalledActivity)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
	}
	username := r.FormValue("username")
	displayName := r.FormValue("displayName")
	password := r.FormValue("password")
	generator := actor.NewPersonGenerator(s.URL, s.KeyGenerator)
	person, err := generator.NewPerson(username, displayName)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create person"), http.StatusBadRequest)
		log.Errorf("Failed to create person, got err=%v", err)
		return
	}
	if err := s.KeyGenerator.WritePrivateKey(filepath.Join(s.Keys, fmt.Sprintf("%s_private.pem", username))); err != nil {
		http.Error(w, fmt.Sprintf("failed to create person"), http.StatusInternalServerError)
		log.Errorf("Failed to write private key, got err=%v", err)
		return
	}
	newUser, err := user.NewUser(username, password, person)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create user"), http.StatusBadRequest)
		log.Errorf("Failed to create user, got err=%v", err)
		return
	}
	if err := s.Datastore.CreateUser(r.Context(), newUser); err != nil {
		http.Error(w, fmt.Sprintf("failed to create user"), http.StatusBadRequest)
		log.Errorf("Failed to create user in datastore, got err=%v", err)
		return
	}
	w.WriteHeader(200)
	w.Write([]byte("Successfully created user and person."))
}

func (s *Server) getActorInbox(w http.ResponseWriter, r *http.Request) {
	actorID := chi.URLParam(r, "username")
	if actorID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	http.Error(w, "actor inbox lookup is unimplemented", http.StatusNotImplemented)
}

func (s *Server) receiveToActorInbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	raw, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errorf("failed to unmarshal JSON from request body: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to unmarshal request body: got err=%v", err), http.StatusBadRequest)
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		log.Errorf("failed to unmarshal JSON from request body: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to unmarshal request body: got err=%v", err), http.StatusBadRequest)
		return
	}
	activityRequest, err := streams.ToType(ctx, m)
	log.Infof("Determining Activity Type: got=%q", activityRequest.GetTypeName())
	switch typeName := activityRequest.GetTypeName(); typeName {
	case "Follow":
		if err := s.follow(ctx, activityRequest); err != nil {
			log.Errorf("Failed to add follower to user: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to accept follower request"), http.StatusInternalServerError)
			return
		}
	case "Undo":
		if err := s.undo(ctx, activityRequest); err != nil {
			log.Errorf("Failed to undo specified Activity: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to undo activity"), http.StatusInternalServerError)
			return
		}
	default:
		log.Errorf("Unsupported Type: got=%q", typeName)
		http.Error(w, "failed to process activityRequest", http.StatusInternalServerError)
		return
	}
	if err := s.Datastore.AddActivityToSharedInbox(r.Context(), activityRequest, s.URL.String()); err != nil {
		log.Errorf("failed to add to inbox: got err=%v", err)
		http.Error(w, "failed to add to inbox", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(200)
}

// Follow allows the actor to follow the requested person on this instance.
// Currently, follow automatically accepts all incoming follow requests, but
// users should be able to confirm and deny follow requests before the Accept
// activity is sent.
func (s *Server) follow(ctx context.Context, activityRequest vocab.Type) error {
	log.Infoln("Received Follow Activity")
	follow, err := follower.ParseFollowRequest(ctx, activityRequest)
	if err != nil {
		return fmt.Errorf("failed to parse follow activityRequest: got err=%v", err)
	}
	followerID := follow.GetActivityStreamsActor().Begin().GetIRI()
	if followerID.String() == "" {
		return fmt.Errorf("follower ID is unspecified: got=%q", followerID)
	}
	actorID := follow.GetActivityStreamsObject().Begin().GetIRI()
	if actorID.String() == "" {
		return fmt.Errorf("actor ID is unspecified: got=%q", actorID)
	}
	accept := follower.PrepareAcceptActivity(follow, actorID)
	marshalledActivity, err := activity.JSON(accept)
	if err != nil {
		return fmt.Errorf("failed to load person: got err=%v", err)
	}
	if err := s.Datastore.AddActivityToSharedInbox(ctx, accept, s.URL.String()); err != nil {
		return fmt.Errorf("failed to add activity to collection: got err=%v", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL.String(), bytes.NewBuffer(marshalledActivity))
	request.Header.Add("Content-Type", `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`)
	if err != nil {
		return fmt.Errorf("failed to create Accept HTTP request: got err=%v", err)
	}
	person, err := s.Datastore.GetActorByActorID(ctx, actorID.String())
	if err != nil {
		return fmt.Errorf("failed to load person: got err=%v", err)
	}
	pem, err := s.readPrivateKey(person.GetActivityStreamsPreferredUsername().GetXMLSchemaString())
	if err != nil {
		return fmt.Errorf("failed to read private key: got err=%v", err)
	}
	privateKey, err := validation.ParsePrivateKeyFromPEMBlock(pem)
	if err != nil {
		return fmt.Errorf("failed to read private key: got err=%v", err)
	}
	request, err = validation.SignRequestWithDigest(request, s.URL, actorID.String(), privateKey)
	if err != nil {
		return fmt.Errorf("failed to sign accept request: got err=%v", err)
	}
	if err := s.Datastore.AddFollowerToActor(ctx, actorID.String(), followerID.String()); err != nil {
		return fmt.Errorf("failed to add follower to actor: got err=%v", err)
	}
	res, err := http.DefaultClient.Do(request)
	defer res.Body.Close()
	if err != nil {
		return fmt.Errorf("failed to send accept request: got err=%v", err)
	}
	body, _ := ioutil.ReadAll(res.Body)
	log.Infof("AcceptActivity successfully POSTed: got=%v StatusCode=%d", string(body), res.StatusCode)
	return nil
}

// Undo can only reverse the effects of Like, Follow, or Block Activities.
// See: https://www.w3.org/TR/activitypub/#undo-activity-outbox
func (s *Server) undo(ctx context.Context, activityRequest vocab.Type) error {
	log.Infoln("Received Undo Activity")
	undoRequest, err := undo.ParseUndoRequest(ctx, activityRequest)
	if err != nil {
		return fmt.Errorf("failed to parse Undo activity: got err=%v", err)
	}
	object := undoRequest.GetActivityStreamsObject()
	if object == nil {
		return fmt.Errorf("failed to receive Object in Undo activity body: got %v", object)
	}
	object.Begin().IsActivityStreamsFollow()
	switch iter := object.Begin(); {
	case iter.IsActivityStreamsFollow():
		follow := object.Begin().GetActivityStreamsFollow()
		followerID := follow.GetActivityStreamsActor().Begin().GetIRI()
		if followerID.String() == "" {
			return fmt.Errorf("follower ID is unspecified: got=%q", followerID)
		}
		actorID := follow.GetActivityStreamsObject().Begin().GetIRI()
		if actorID.String() == "" {
			return fmt.Errorf("actor ID is unspecified: got=%q", actorID)
		}
		if err := s.Datastore.RemoveFollowerFromActor(ctx, actorID.String(), followerID.String()); err != nil {
			return fmt.Errorf("failed to remove follower: got err=%v", err)
		}
	case iter.IsActivityStreamsLike():
		return fmt.Errorf("undo like activity support is unimplemented")
	case iter.IsActivityStreamsBlock():
		return fmt.Errorf("undo block activity support is unimplemented")
	default:
		return fmt.Errorf("activity is unsupported in the ActivityPub specification")
	}
	return nil
}

func (s *Server) getActorOutbox(w http.ResponseWriter, r *http.Request) {
	actorID := chi.URLParam(r, "username")
	if actorID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	http.Error(w, "actor outbox lookup is unimplemented", http.StatusNotImplemented)
}

// webfinger allow other services to query if a user on this instance.
func (s *Server) webfinger(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Query().Get("resource")
	var username string
	// Find alternative to this parsing (other than Regex)
	splitString := strings.Split(resource, "@"+s.URL.Host)
	if len(splitString) == 0 {
		http.Error(w, fmt.Sprintf("invalid resource string"), http.StatusBadRequest)
		return
	}
	_, err := fmt.Sscanf(splitString[0], "acct:%s", &username)
	if err != nil {
		log.Errorf("failed to parse username=%q from resource=%q: got err=%v", username, resource, err)
		http.Error(w, fmt.Sprintf("failed to parse username"), http.StatusBadRequest)
		return
	}
	person, err := s.Datastore.GetActorByUsername(r.Context(), username)
	if err != nil {
		http.Error(w, fmt.Sprintf("unable to find actor"), http.StatusNotFound)
		return
	}
	id := person.GetJSONLDId().Get()
	res := &WebfingerResponse{
		Subject: resource,
		Links: []WebfingerLink{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: id.String(),
			},
		},
	}
	response, err := json.Marshal(res)
	if err != nil {
		log.Errorf("failed to get actor with username=%q: got err=%v", username, err)
		http.Error(w, "failed to load actor", http.StatusInternalServerError)
	}
	w.WriteHeader(200)
	w.Write(response)
}

func (s *Server) readPrivateKey(actor string) (string, error) {
	path := filepath.Join(s.Keys, fmt.Sprintf("%s_private.pem", actor))
	log.Infof("Loading Path=%q", path)
	privateKeyPEM, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(privateKeyPEM), nil
}
