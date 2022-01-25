package activitypub

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/FediUni/FediUni/activitypub/client"
	"github.com/FediUni/FediUni/activitypub/follower"
	"github.com/FediUni/FediUni/activitypub/undo"
	"github.com/FediUni/FediUni/activitypub/validation"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-chi/jwtauth"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	"github.com/go-redis/redis"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"

	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/FediUni/FediUni/activitypub/user"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	log "github.com/golang/glog"
)

type Datastore interface {
	GetActorByUsername(context.Context, string) (actor.Person, error)
	GetActivityByObjectID(context.Context, string, string) (vocab.Type, error)
	GetActivityByActivityID(context.Context, string) (vocab.Type, error)
	CreateUser(context.Context, *user.User) error
	GetUserByUsername(context.Context, string) (*user.User, error)
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
	Redis        *redis.Client
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

var (
	tokenAuth *jwtauth.JWTAuth
)

func NewServer(instanceURL, keys string, datastore Datastore, keyGenerator actor.KeyGenerator, secret string) (*Server, error) {
	url, err := url.Parse(instanceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse instanceURL=%q: got err=%v", instanceURL, err)
	}
	tokenAuth = jwtauth.New("HS256", []byte(secret), nil)
	s := &Server{
		URL:          url,
		Keys:         keys,
		Datastore:    datastore,
		KeyGenerator: keyGenerator,
		Client:       client.NewClient(url),
		Redis: redis.NewClient(&redis.Options{
			Addr:     "redis:6379",
			Password: viper.GetString("REDIS_PASSWORD"),
		}),
	}
	s.Router = chi.NewRouter()

	s.Router.Use(middleware.RealIP)
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
	s.Router.Post("/login", s.login)
	s.Router.With(jwtauth.Verifier(tokenAuth)).Post("/follow", s.sendFollowRequest)
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
	activity, err := s.Datastore.GetActivityByObjectID(r.Context(), activityID, s.URL.String())
	if err != nil {
		log.Errorf("failed to get activity with ID=%q: got err=%v", activityID, err)
		http.Error(w, "failed to load activity", http.StatusNotFound)
		return
	}
	serializedActivity, err := streams.Serialize(activity)
	if err != nil {
		log.Errorf("failed to serialize activity with ID=%q: got err=%v", activityID, err)
		http.Error(w, "failed to load activity", http.StatusNotFound)
		return
	}
	marshalledActivity, err := json.Marshal(serializedActivity)
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
	newUser, err := user.NewUser(username, password, person)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create user"), http.StatusBadRequest)
		log.Errorf("Failed to create user, got err=%v", err)
		return
	}
	privateKeyPEM, err := s.KeyGenerator.GetPrivateKeyPEM()
	if err != nil {
		log.Errorf("Failed to get private key: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to create person"), http.StatusInternalServerError)
		return
	}
	if err := s.Datastore.CreateUser(r.Context(), newUser); err != nil {
		http.Error(w, fmt.Sprintf("failed to create user"), http.StatusBadRequest)
		log.Errorf("Failed to create user in datastore, got err=%v", err)
		return
	}
	if err := s.Redis.Set(strings.ToLower(username), privateKeyPEM, time.Duration(0)).Err(); err != nil {
		log.Errorf("Failed to write private key: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to create person"), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(200)
	w.Write([]byte("Successfully created user and person."))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		log.Errorf("failed to parse Login Form: got err=%v", err)
		http.Error(w, fmt.Sprint("failed to parse login form"), http.StatusBadRequest)
		return
	}
	username := r.FormValue("username")
	if username == "" {
		log.Errorf("failed to parse Login Form: got username=%q", username)
		http.Error(w, fmt.Sprint("failed to parse login form: invalid username"), http.StatusBadRequest)
		return
	}
	password := r.FormValue("password")
	if password == "" {
		log.Errorf("failed to parse Login Form: got password=%q", password)
		http.Error(w, fmt.Sprint("failed to parse login form: invalid password"), http.StatusBadRequest)
		return
	}
	user, err := s.Datastore.GetUserByUsername(r.Context(), username)
	if err != nil {
		log.Errorf("failed to load username=%q details: got err=%v", username, err)
		http.Error(w, fmt.Sprint("failed to load user details"), http.StatusInternalServerError)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		log.Errorf("invalid password provided: got err=%v", err)
		http.Error(w, fmt.Sprint("invalid password"), http.StatusInternalServerError)
		return
	}
	expirationTime := time.Now().Add(72 * time.Hour)
	token, err := createToken(username, expirationTime)
	if err != nil {
		log.Errorf("failed to generate JWT: got err=%v", err)
		http.Error(w, fmt.Sprint("failed to generate JWT"), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    token,
		Expires:  expirationTime,
		HttpOnly: true,
	})
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
	log.Infof("Incoming Request Headers=%v", r.Header)
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
		if err := s.handleFollowRequest(ctx, activityRequest); err != nil {
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
	case "Accept":
		if err := s.handleAccept(ctx, activityRequest); err != nil {
			log.Errorf("Failed to handle Accept Activity: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to handle Accept activity"), http.StatusInternalServerError)
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

func (s *Server) sendFollowRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, claims, err := jwtauth.FromContext(r.Context())
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		return
	}
	rawUsername := claims["username"]
	var username string
	switch rawUsername.(type) {
	case string:
		username = rawUsername.(string)
	default:
		log.Errorf("Invalid actor ID presented: got %v", rawUsername)
		http.Error(w, fmt.Sprintf("failed to load username"), http.StatusUnauthorized)
		return
	}
	raw, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Failed to read request body: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to read body"), http.StatusInternalServerError)
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		log.Errorf("Failed to unmarshal follow request: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to unmarshal follow request"), http.StatusInternalServerError)
		return
	}
	rawActorID := m["actorID"]
	var actorToFollow string
	switch rawActorID.(type) {
	case string:
		actorToFollow = rawActorID.(string)
	default:
		log.Errorf("Invalid actor ID presented: got %v", rawActorID)
		http.Error(w, fmt.Sprintf("invalid follow request presented: expected @username@domain format"), http.StatusBadRequest)
		return
	}
	split := strings.SplitN(actorToFollow, "@", 3)
	usernameToFollow := split[1]
	domain := split[2]
	if username == "" || domain == "" {
		log.Errorf("invalid follow request presented: got actorID=%v", split)
		http.Error(w, fmt.Sprintf("invalid follow request presented: expected @username@domain format"), http.StatusBadRequest)
		return
	}
	webfingerURL, err := url.Parse(fmt.Sprintf("https://%s/.well-known/webfinger", domain))
	if err != nil {
		log.Errorf("failed to parse URL from domain=%q: got err=%v", domain, err)
		http.Error(w, fmt.Sprintf("invalid follow request presented: invalid domain presented"), http.StatusBadRequest)
		return
	}
	res, err := s.Client.WebfingerLookup(ctx, webfingerURL, usernameToFollow)
	if err != nil {
		log.Errorf("failed to perform webfinger lookup: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to perform webfinger lookup"), http.StatusInternalServerError)
		return
	}
	log.Infof("Received %q", res)
	var webfingerResponse WebfingerResponse
	if err := json.Unmarshal(res, &webfingerResponse); err != nil {
		log.Errorf("failed to perform webfinger lookup: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to perform webfinger lookup"), http.StatusInternalServerError)
		return
	}
	if len(webfingerResponse.Links) == 0 {
		log.Errorf("failed to perform webfinger lookup: got number_of_links=%d", len(webfingerResponse.Links))
		http.Error(w, fmt.Sprintf("failed to perform webfinger lookup"), http.StatusInternalServerError)
		return
	}
	var actorID *url.URL
	for _, link := range webfingerResponse.Links {
		if !strings.Contains(link.Type, "application/activity+json") && !strings.Contains(link.Type, "application/ld+json") {
			continue
		}
		if actorID, err = url.Parse(link.Href); err != nil {
			log.Errorf("failed to load actorID: got err=%v", err)
			http.Error(w, fmt.Sprintf("failed to retrieve actor=%q", actorToFollow), http.StatusBadRequest)
			return
		}
		break
	}
	object, err := s.Client.FetchRemoteObject(ctx, actorID)
	var personToFollow vocab.ActivityStreamsPerson
	resolver, err := streams.NewTypeResolver(func(ctx context.Context, p vocab.ActivityStreamsPerson) error {
		personToFollow = p
		return nil
	})
	if err != nil {
		log.Errorf("failed to create Type Resolver: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to retrieve actor=%q", actorToFollow), http.StatusInternalServerError)
		return
	}
	if object == nil {
		log.Errorf("failed to fetch remote object: got %v", object)
		http.Error(w, fmt.Sprintf("failed to resolve remote actor=%q", actorToFollow), http.StatusInternalServerError)
		return
	}
	if err := resolver.Resolve(ctx, object); err != nil {
		log.Errorf("failed to resolve actor=%q: got err=%v", actorToFollow, err)
		http.Error(w, fmt.Sprintf("failed to retrieve actor=%q", actorToFollow), http.StatusInternalServerError)
		return
	}
	inbox := personToFollow.GetActivityStreamsInbox()
	if inbox == nil {
		log.Errorf("actor=%q failed to provide inbox", actorToFollow)
		http.Error(w, fmt.Sprintf("actor=%q failed to provide an inbox", actorToFollow), http.StatusInternalServerError)
		return
	}
	person, err := s.Datastore.GetActorByUsername(ctx, username)
	if err != nil {
		log.Errorf("failed to load actor=%q: got err=%v", username, err)
		http.Error(w, fmt.Sprintf("failed to load actor=%q", username), http.StatusInternalServerError)
		return
	}
	inboxURL := inbox.GetIRI()
	followActivity := streams.NewActivityStreamsFollow()
	actorProperty := streams.NewActivityStreamsActorProperty()
	actorProperty.AppendActivityStreamsPerson(personToFollow)
	followActivity.SetActivityStreamsActor(actorProperty)
	objectProperty := streams.NewActivityStreamsObjectProperty()
	objectProperty.AppendActivityStreamsPerson(person)
	followActivity.SetActivityStreamsObject(objectProperty)
	if err := s.Datastore.AddActivityToSharedInbox(ctx, followActivity, s.URL.String()); err != nil {
		log.Errorf("Failed to add Follow Activity to Datastore: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to send follow request"), http.StatusInternalServerError)
	}
	privateKey, err := s.readPrivateKey(username)
	if err != nil {
		log.Errorf("failed to read private key: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to send follow request"), http.StatusInternalServerError)
		return
	}
	if err := s.Client.PostToInbox(ctx, inboxURL, followActivity, person.GetJSONLDId().Get().String(), privateKey); err != nil {
		log.Errorf("failed to post to inbox: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to send follow request"), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	return
}

// handleFollowRequest allows the actor to follow the requested person on this instance.
// Currently, follow automatically accepts all incoming handleFollowRequest requests, but
// users should be able to confirm and deny handleFollowRequest requests before the Accept
// activity is sent.
func (s *Server) handleFollowRequest(ctx context.Context, activityRequest vocab.Type) error {
	log.Infoln("Received Follow Activity")
	follow, err := follower.ParseFollowRequest(ctx, activityRequest)
	if err != nil {
		return fmt.Errorf("failed to parse handleFollowRequest activityRequest: got err=%v", err)
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
	// Ensure Actor exists on this server before adding activity.
	person, err := s.Datastore.GetActorByActorID(ctx, actorID.String())
	if err != nil {
		return fmt.Errorf("failed to load person: got err=%v", err)
	}
	if err := s.Datastore.AddActivityToSharedInbox(ctx, accept, s.URL.String()); err != nil {
		return fmt.Errorf("failed to add activity to collection: got err=%v", err)
	}
	object, err := s.Client.FetchRemoteObject(ctx, followerID)
	if err != nil {
		return err
	}
	var remoteActor actor.Person
	resolver, err := streams.NewTypeResolver(func(ctx context.Context, person vocab.ActivityStreamsPerson) error {
		remoteActor = person
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create a Person resolver: got err=%v", err)
	}
	if err = resolver.Resolve(ctx, object); err != nil {
		return fmt.Errorf("failed to resolve vocab.Type to Person: got err=%v", err)
	}
	if remoteActor.GetActivityStreamsInbox() == nil {
		return fmt.Errorf("invalid actor=%q retrieved! inbox property does not exist", followerID.String())
	}
	privateKey, err := s.readPrivateKey(person.GetActivityStreamsPreferredUsername().GetXMLSchemaString())
	if err != nil {
		return fmt.Errorf("failed to read private key: got err=%v", err)
	}
	log.Infof("Sending Accept Request to %q", remoteActor.GetActivityStreamsInbox().GetIRI().String())
	if err := s.Client.PostToInbox(ctx, remoteActor.GetActivityStreamsInbox().GetIRI(), accept, actorID.String(), privateKey); err != nil {
		return fmt.Errorf("failed to post to actor Inbox=%q: got err=%v", remoteActor.GetActivityStreamsInbox().GetIRI().String(), err)
	}
	if err := s.Datastore.AddFollowerToActor(ctx, actorID.String(), followerID.String()); err != nil {
		return fmt.Errorf("failed to add follower to actor: got err=%v", err)
	}
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

func (s *Server) handleAccept(ctx context.Context, activityRequest vocab.Type) error {
	accept, err := follower.ParseAcceptActivity(ctx, activityRequest)
	if err != nil {
		return err
	}
	presentedObject := accept.GetActivityStreamsObject()
	if presentedObject.Empty() {
		return fmt.Errorf("failed to receive an Object in Accept Activity: got=%v", presentedObject)
	}
	objectID := presentedObject.Begin().GetIRI()
	loadedObject, err := s.Datastore.GetActivityByActivityID(ctx, objectID.String())
	if err != nil {
		return fmt.Errorf("failed to load follow Activity=%q: got err=%v", objectID.String(), err)
	}
	follow, err := follower.ParseFollowRequest(ctx, loadedObject)
	if err != nil {
		return fmt.Errorf("failed parse follow Activity: got err=%v", err)
	}
	followerID := follow.GetActivityStreamsActor().Begin().GetIRI()
	if followerID.String() == "" {
		return fmt.Errorf("follower ID is unspecified: got=%q", followerID)
	}
	actorID := follow.GetActivityStreamsObject().Begin().GetIRI()
	if actorID.String() == "" {
		return fmt.Errorf("actor ID is unspecified: got=%q", actorID)
	}
	if err := s.Datastore.AddFollowerToActor(ctx, actorID.String(), followerID.String()); err != nil {
		return fmt.Errorf("failed to add Follower=%q to Actor=%q", followerID.String(), actorID.String())
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

func (s *Server) readPrivateKey(actor string) (*rsa.PrivateKey, error) {
	privateKeyPEM, err := s.Redis.Get(strings.ToLower(actor)).Result()
	if err != nil {
		return nil, err
	}
	privateKey, err := validation.ParsePrivateKeyFromPEMBlock(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	if err := privateKey.Validate(); err != nil {
		return nil, err
	}
	return privateKey, err
}

func createToken(username string, expirationTime time.Time) (string, error) {
	claims := jwt.MapClaims{}
	claims["username"] = username
	claims["exp"] = expirationTime.Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	secret := viper.GetString("SECRET")
	if secret == "" {
		return "", fmt.Errorf("failed to provide a secret for JWT signing")
	}
	return token.SignedString([]byte(secret))
}
