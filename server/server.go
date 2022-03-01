package server

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

	"github.com/FediUni/FediUni/server/activity"
	"github.com/FediUni/FediUni/server/actor"
	"github.com/FediUni/FediUni/server/client"
	"github.com/FediUni/FediUni/server/follower"
	"github.com/FediUni/FediUni/server/undo"
	"github.com/FediUni/FediUni/server/user"
	"github.com/FediUni/FediUni/server/validation"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/go-chi/jwtauth"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	"github.com/go-redis/redis"
	log "github.com/golang/glog"
	"github.com/microcosm-cc/bluemonday"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

type Datastore interface {
	GetActorByUsername(context.Context, string) (actor.Person, error)
	GetActivityByObjectID(context.Context, string, string) (vocab.Type, error)
	GetActivityByActivityID(context.Context, string) (vocab.Type, error)
	GetFollowersByUsername(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error)
	GetFollowingByUsername(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error)
	GetFollowerStatus(context.Context, string, string) (int, error)
	CreateUser(context.Context, *user.User) error
	GetUserByUsername(context.Context, string) (*user.User, error)
	AddActivityToActorInbox(context.Context, vocab.Type, string, bool) error
	AddActivityToPublicInbox(context.Context, vocab.Type, primitive.ObjectID) error
	AddActivityToOutbox(context.Context, vocab.Type, string) error
	AddFollowerToActor(context.Context, string, string) error
	AddActorToFollows(context.Context, string, string) error
	RemoveFollowerFromActor(context.Context, string, string) error
	GetActorByActorID(context.Context, string) (actor.Person, error)
	AddObjectsToActorInbox(context.Context, []vocab.Type, string) error
	GetActorInbox(context.Context, string, string, string) (vocab.ActivityStreamsOrderedCollectionPage, error)
	GetActorInboxAsOrderedCollection(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error)
	GetActorOutbox(context.Context, string, string, string) (vocab.ActivityStreamsOrderedCollectionPage, error)
	GetActorOutboxAsOrderedCollection(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error)
}

type Server struct {
	URL          *url.URL
	Router       *chi.Mux
	Datastore    Datastore
	Redis        *redis.Client
	KeyGenerator actor.KeyGenerator
	Client       *client.Client
	Policy       *bluemonday.Policy
	Secret       string
}

var (
	tokenAuth *jwtauth.JWTAuth
)

func New(instanceURL *url.URL, datastore Datastore, keyGenerator actor.KeyGenerator, secret string) (*Server, error) {
	tokenAuth = jwtauth.New("HS256", []byte(secret), nil)
	s := &Server{
		URL:          instanceURL,
		Datastore:    datastore,
		KeyGenerator: keyGenerator,
		Client:       client.NewClient(instanceURL, "redis:6379", viper.GetString("REDIS_PASSWORD")),
		Redis: redis.NewClient(&redis.Options{
			Addr:     "redis:6379",
			Password: viper.GetString("REDIS_PASSWORD"),
		}),
		Policy: bluemonday.UGCPolicy(),
		Secret: secret,
	}

	s.Router = chi.NewRouter()
	s.Router.Use(middleware.RealIP)
	s.Router.Use(middleware.Logger)
	s.Router.Use(middleware.Timeout(60 * time.Second))
	s.Router.Use(httprate.LimitAll(100, time.Minute*1))
	s.Router.Use(cors.Handler(cors.Options{
		AllowOriginFunc:  func(r *http.Request, origin string) bool { return true },
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	activitypubRouter := chi.NewRouter()
	activitypubRouter.Get("/actor", s.getAnyActor)
	activitypubRouter.Get("/actor/{username}", s.getActor)
	activitypubRouter.Get("/actor/outbox", s.getAnyActorOutbox)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Get("/actor/{username}/inbox", s.getActorInbox)
	activitypubRouter.With(validation.Signature).Post("/actor/{username}/inbox", s.receiveToActorInbox)
	activitypubRouter.Get("/actor/{username}/outbox", s.getActorOutbox)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Post("/actor/{username}/outbox", s.postActorOutbox)
	activitypubRouter.Get("/actor/{username}/followers", s.getFollowers)
	activitypubRouter.Get("/actor/{username}/following", s.getFollowing)
	activitypubRouter.Get("/activity/{activityID}", s.getActivity)
	activitypubRouter.Post("/register", s.createUser)
	activitypubRouter.Post("/login", s.login)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Post("/follow", s.sendFollowRequest)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Post("/follow/status", s.checkFollowStatus)

	s.Router.Get("/.well-known/webfinger", s.Webfinger)
	s.Router.Mount("/api", activitypubRouter)
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
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

func (s *Server) getAnyActor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identifier := r.URL.Query().Get("identifier")
	if identifier == "" {
		log.Errorf("failed to receive identifier: got=%q", identifier)
		http.Error(w, "Failed to receive an actor identifier", http.StatusBadRequest)
		return
	}
	actor, err := s.Client.FetchRemoteActor(ctx, identifier)
	if err != nil {
		log.Errorf("failed to retrieve actor: got err=%v", err)
		http.Error(w, "Failed to load actor", http.StatusInternalServerError)
		return
	}
	m, err := streams.Serialize(actor)
	if err != nil {
		log.Errorf("failed to serialize actor: got err=%v", err)
		http.Error(w, "Failed to load actor", http.StatusInternalServerError)
		return
	}
	marshalledActivity, err := json.Marshal(m)
	if err != nil {
		log.Errorf("failed to marshal actor to JSON: got err=%v", err)
		http.Error(w, "Failed to load actor", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.Write(marshalledActivity)
}

func (s *Server) getAnyActorOutbox(w http.ResponseWriter, r *http.Request) {
	identifier := r.URL.Query().Get("identifier")
	if identifier == "" {
		log.Errorf("failed to receive identifier: got=%q", identifier)
		http.Error(w, "Failed to receive an actor identifier", http.StatusBadRequest)
		return
	}
	actor, err := s.Client.FetchRemoteActor(r.Context(), identifier)
	if err != nil {
		log.Errorf("failed to fetch a remote actor=%q: got err=%v", identifier, err)
		http.Error(w, "Failed to load actor", http.StatusNotFound)
		return
	}
	var person vocab.ActivityStreamsPerson
	resolver, err := streams.NewTypeResolver(func(ctx context.Context, p vocab.ActivityStreamsPerson) error {
		person = p
		return nil
	})
	if err != nil {
		log.Errorf("failed to create Actor Resolver: got err=%v", err)
		http.Error(w, "Failed to load actor", http.StatusNotFound)
		return
	}
	if err := resolver.Resolve(r.Context(), actor); err != nil {
		log.Errorf("failed to resolve Actor: got err=%v", err)
		http.Error(w, "Failed to load actor", http.StatusNotFound)
		return
	}
	outbox := person.GetActivityStreamsOutbox()
	if outbox == nil {
		log.Errorf("failed to load outbox: got=%v", outbox)
		http.Error(w, "Failed to load outbox", http.StatusNotFound)
		return
	}
	outboxIRI := outbox.GetIRI()
	if outboxIRI == nil {
		log.Errorf("failed to load outbox URL: got=%v", outboxIRI)
		http.Error(w, "Failed to load outbox", http.StatusNotFound)
		return
	}
	// TODO(Jared-Mullin): We should define a function that loads the
	// OrderedCollection and then the OrderedCollectionPage rather than guessing
	// ?page=true will always hold for outbox.
	outboxPageIRI, err := url.Parse(fmt.Sprintf("%s?page=true", outboxIRI.String()))
	if err != nil {
		log.Errorf("failed to load parse outbox URL: got=%v", outboxIRI)
		http.Error(w, "Failed to load outbox", http.StatusNotFound)
		return
	}
	outboxPage, err := s.Client.FetchRemoteObject(r.Context(), outboxPageIRI, false)
	if err != nil {
		log.Errorf("failed to load outbox: got=%v", outbox)
		http.Error(w, "Failed to load outbox", http.StatusNotFound)
		return
	}
	var page vocab.ActivityStreamsOrderedCollectionPage
	resolver, err = streams.NewTypeResolver(func(ctx context.Context, p vocab.ActivityStreamsOrderedCollectionPage) error {
		page = p
		return nil
	})
	if err != nil {
		log.Errorf("failed to create type resolver: got err=%v", err)
		http.Error(w, "Failed to load outbox", http.StatusNotFound)
		return
	}
	if err := resolver.Resolve(r.Context(), outboxPage); err != nil {
		log.Errorf("failed to resolve %v: got err=%v", outboxPage, err)
		http.Error(w, "Failed to load outbox", http.StatusNotFound)
		return
	}
	m, err := streams.Serialize(page)
	if err != nil {
		log.Errorf("failed to serialize %v: got err=%v", outboxPage, err)
		http.Error(w, "Failed to load outbox", http.StatusNotFound)
		return
	}
	serializedOutbox, err := json.Marshal(m)
	if err != nil {
		log.Errorf("failed to serialize %v: got err=%v", m, err)
		http.Error(w, "Failed to load outbox", http.StatusNotFound)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.Write(serializedOutbox)
	return
}

func (s *Server) getFollowers(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if username == "" {
		http.Error(w, "username is unspecified", http.StatusBadRequest)
		return
	}
	followers, err := s.Datastore.GetFollowersByUsername(r.Context(), username)
	if err != nil {
		log.Errorf("failed to load followers from Datastore: got err=%v", err)
		http.Error(w, "failed to load followers", http.StatusInternalServerError)
		return
	}
	m, err := streams.Serialize(followers)
	if err != nil {
		log.Errorf("failed to serialize activity : got err=%v", err)
		http.Error(w, "failed to load followers", http.StatusInternalServerError)
		return
	}
	marshalledActivity, err := json.Marshal(m)
	if err != nil {
		log.Errorf("failed to marshal activity to JSON: got err=%v", err)
		http.Error(w, "failed to load followers", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.Write(marshalledActivity)
}

func (s *Server) getFollowing(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if username == "" {
		http.Error(w, "username is unspecified", http.StatusBadRequest)
		return
	}
	followers, err := s.Datastore.GetFollowingByUsername(r.Context(), username)
	if err != nil {
		log.Errorf("failed to load followers from Datastore: got err=%v", err)
		http.Error(w, "failed to load followers", http.StatusInternalServerError)
		return
	}
	m, err := streams.Serialize(followers)
	if err != nil {
		log.Errorf("failed to serialize activity : got err=%v", err)
		http.Error(w, "failed to load followers", http.StatusInternalServerError)
		return
	}
	marshalledActivity, err := json.Marshal(m)
	if err != nil {
		log.Errorf("failed to marshal activity to JSON: got err=%v", err)
		http.Error(w, "failed to load followers", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.Write(marshalledActivity)
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
	w.Header().Add("Content-Type", "application/activity+json")

	w.WriteHeader(http.StatusOK)
	w.Write(marshalledActivity)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	// Set MaxMemory to 8MB.
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		log.Errorf("failed to parse Login Form: got err=%v", err)
		http.Error(w, fmt.Sprint("failed to create user"), http.StatusBadRequest)
		return
	}
	username := r.FormValue("username")
	if username == "" {
		log.Errorf("failed to create user: username is unspecified")
		http.Error(w, "username is unspecified", http.StatusBadRequest)
	}
	displayName := r.FormValue("name")
	password := r.FormValue("password")
	if password == "" {
		log.Errorf("failed to create user: password is unspecified")
		http.Error(w, "password is unspecified", http.StatusBadRequest)
	}
	generator := actor.NewPersonGenerator(s.URL, s.KeyGenerator)
	person, err := generator.NewPerson(username, displayName)
	if err != nil {
		log.Errorf("Failed to create person, got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to create user"), http.StatusInternalServerError)
		return
	}
	newUser, err := user.NewUser(username, password, person)
	if err != nil {
		log.Errorf("Failed to create user, got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to create user"), http.StatusInternalServerError)
		return
	}
	privateKeyPEM, err := s.KeyGenerator.GetPrivateKeyPEM()
	if err != nil {
		log.Errorf("Failed to get private key: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to create user"), http.StatusInternalServerError)
		return
	}
	if err := s.Datastore.CreateUser(r.Context(), newUser); err != nil {
		log.Errorf("Failed to create user in datastore, got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to create user"), http.StatusInternalServerError)
		return
	}
	if err := s.Redis.Set(strings.ToLower(username), privateKeyPEM, time.Duration(0)).Err(); err != nil {
		log.Errorf("Failed to write private key: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to create user"), http.StatusInternalServerError)
		return
	}
	res, err := s.generateTokenResponse(username)
	if err != nil {
		log.Errorf("Failed to create token response: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to create user"), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(res)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	// Set MaxMemory to 8MB.
	if err := r.ParseMultipartForm(8 << 20); err != nil {
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
	res, err := s.generateTokenResponse(username)
	if err != nil {
		log.Errorf("failed to generate token response: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to authenticate user: got err=%v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write(res)
}

func (s *Server) getActorInbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := chi.URLParam(r, "username")
	if username == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_, _, err := jwtauth.FromContext(ctx)
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to receive JWT"), http.StatusUnauthorized)
		return
	}
	page := r.URL.Query().Get("page")
	if strings.ToLower(page) != "true" {
		orderedCollection, err := s.Datastore.GetActorInboxAsOrderedCollection(ctx, username)
		if err != nil {
			log.Errorf("Failed to read from Inbox of Username=%q: got err=%v", username, err)
			http.Error(w, fmt.Sprintf("failed to load actor inbox"), http.StatusInternalServerError)
			return
		}
		serializedOrderedCollection, err := streams.Serialize(orderedCollection)
		if err != nil {
			log.Errorf("Failed to serialize Ordered Collection Inbox of Username=%q: got err=%v", username, err)
			http.Error(w, fmt.Sprintf("failed to load actor inbox"), http.StatusInternalServerError)
			return
		}
		marshalledOrderedCollection, err := json.Marshal(serializedOrderedCollection)
		if err != nil {
			log.Errorf("Failed to marshal Ordered Collection Inbox of Username=%q: got err=%v", username, err)
			http.Error(w, fmt.Sprintf("failed to load actor inbox"), http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Type", "application/activity+json")
		w.WriteHeader(http.StatusOK)
		w.Write(marshalledOrderedCollection)
		return
	}
	maxID := r.URL.Query().Get("max_id")
	if maxID == "" {
		maxID = "0"
	}
	minID := r.URL.Query().Get("min_id")
	if minID == "" {
		minID = "0"
	}
	orderedCollectionPage, err := s.Datastore.GetActorInbox(ctx, username, minID, maxID)
	if err != nil {
		log.Errorf("Failed to read from Inbox of Username=%q: got err=%v", username, err)
		http.Error(w, fmt.Sprintf("failed to load actor inbox"), http.StatusInternalServerError)
		return
	}
	serializedOrderedCollection, err := streams.Serialize(orderedCollectionPage)
	if err != nil {
		log.Errorf("Failed to serialize Ordered Collection Page Inbox of Username=%q: got err=%v", username, err)
		http.Error(w, fmt.Sprintf("failed to load actor inbox page"), http.StatusInternalServerError)
		return
	}
	marshalledOrderedCollection, err := json.Marshal(serializedOrderedCollection)
	if err != nil {
		log.Errorf("Failed to marshal Ordered Collection Page Inbox of Username=%q: got err=%v", username, err)
		http.Error(w, fmt.Sprintf("failed to load actor inbox page"), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(marshalledOrderedCollection)
}

// getActorOutbox returns activities posted by the actor as OrderedCollectionPages.
// When the "page" query parameter is unset, an OrderedCollection is returned.
// When the max_id or min_id is set, activities with an object ID strictly less
// than or greater than the specified ObjectID are returned.
func (s *Server) getActorOutbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := chi.URLParam(r, "username")
	if username == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	page := r.URL.Query().Get("page")
	if strings.ToLower(page) != "true" {
		orderedCollection, err := s.Datastore.GetActorOutboxAsOrderedCollection(ctx, username)
		if err != nil {
			log.Errorf("Failed to read from Inbox of Actor ID=%q: got err=%v", username, err)
			http.Error(w, fmt.Sprintf("failed to load actor inbox"), http.StatusInternalServerError)
			return
		}
		serializedOrderedCollection, err := streams.Serialize(orderedCollection)
		if err != nil {
			log.Errorf("Failed to serialize Ordered Collection Inbox of Actor ID=%q: got err=%v", username, err)
			http.Error(w, fmt.Sprintf("failed to load actor inbox"), http.StatusInternalServerError)
			return
		}
		marshalledOrderedCollection, err := json.Marshal(serializedOrderedCollection)
		if err != nil {
			log.Errorf("Failed to marshal Ordered Collection Inbox of Actor ID=%q: got err=%v", username, err)
			http.Error(w, fmt.Sprintf("failed to load actor inbox"), http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Type", "application/activity+json")
		w.WriteHeader(http.StatusOK)
		w.Write(marshalledOrderedCollection)
		return
	}
	maxID := r.URL.Query().Get("max_id")
	if maxID == "" {
		maxID = "0"
	}
	minID := r.URL.Query().Get("min_id")
	if minID == "" {
		minID = "0"
	}
	orderedCollectionPage, err := s.Datastore.GetActorOutbox(ctx, username, minID, maxID)
	if err != nil {
		log.Errorf("Failed to read from Inbox of Actor ID=%q: got err=%v", username, err)
		http.Error(w, fmt.Sprintf("failed to load actor inbox"), http.StatusInternalServerError)
		return
	}
	serializedOrderedCollection, err := streams.Serialize(orderedCollectionPage)
	if err != nil {
		log.Errorf("Failed to serialize Ordered Collection Page Inbox of Actor ID=%q: got err=%v", username, err)
		http.Error(w, fmt.Sprintf("failed to load actor inbox page"), http.StatusInternalServerError)
		return
	}
	marshalledOrderedCollection, err := json.Marshal(serializedOrderedCollection)
	if err != nil {
		log.Errorf("Failed to marshal Ordered Collection Page Inbox of Actor ID=%q: got err=%v", username, err)
		http.Error(w, fmt.Sprintf("failed to load actor inbox page"), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(marshalledOrderedCollection)
}

func (s *Server) postActorOutbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := chi.URLParam(r, "username")
	if username == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_, claims, err := jwtauth.FromContext(r.Context())
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	userID, err := parseJWTActorID(claims)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	raw, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	object, err := streams.ToType(ctx, m)
	if err != nil {
		log.Errorf("failed to convert JSON to activitypub type: got err=%v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	identifier := fmt.Sprintf("@%s@%s", username, s.URL.Host)
	person, err := s.Client.FetchRemotePerson(ctx, identifier)
	if err != nil {
		http.Error(w, fmt.Sprintf("actor does not exist"), http.StatusInternalServerError)
		return
	}
	switch typeName := object.GetTypeName(); typeName {
	case "Note":
		var note vocab.ActivityStreamsNote
		noteResolver, err := streams.NewTypeResolver(func(ctx context.Context, n vocab.ActivityStreamsNote) error {
			note = n
			return nil
		})
		if err != nil {
			log.Errorf("failed to create Note resolver: got err=%v", err)
			http.Error(w, fmt.Sprintf("failed to send Note"), http.StatusInternalServerError)
			return
		}
		if err := noteResolver.Resolve(ctx, object); err != nil {
			log.Errorf("failed to resolve object to note: got err=%v", err)
			http.Error(w, fmt.Sprintf("type mismatch: object is not a note"), http.StatusBadRequest)
			return
		}
		for iter := note.GetActivityStreamsAttributedTo().Begin(); iter != nil; iter = iter.Next() {
			if !iter.IsIRI() {
				continue
			}
			if iter.GetIRI().String() != userID {
				http.Error(w, fmt.Sprintf("attributedTo mismatch in note"), http.StatusUnauthorized)
				return
			}
		}
		objectID := primitive.NewObjectID()
		id, err := url.Parse(fmt.Sprintf("%s/activity/%s", s.URL.String(), objectID.Hex()))
		if err != nil {
			log.Errorf("failed to add activities to datastore: got err=%v", err)
			http.Error(w, fmt.Sprintf("failed to post activity"), http.StatusInternalServerError)
			return
		}
		idProperty := streams.NewJSONLDIdProperty()
		idProperty.Set(id)
		create := streams.NewActivityStreamsCreate()
		create.SetJSONLDId(idProperty)
		note.SetJSONLDId(idProperty)
		object := streams.NewActivityStreamsObjectProperty()
		object.AppendActivityStreamsNote(note)
		create.SetActivityStreamsObject(object)
		actor := streams.NewActivityStreamsActorProperty()
		actor.AppendActivityStreamsPerson(person)
		create.SetActivityStreamsActor(actor)
		create.SetActivityStreamsPublished(note.GetActivityStreamsPublished())
		create.SetActivityStreamsTo(note.GetActivityStreamsTo())
		create.SetActivityStreamsCc(note.GetActivityStreamsCc())
		if err := s.Datastore.AddActivityToPublicInbox(ctx, create, objectID); err != nil {
			log.Errorf("failed to add activities to datastore: got err=%v", err)
			http.Error(w, fmt.Sprintf("failed to post activity"), http.StatusInternalServerError)
			return
		}
		if err := s.Datastore.AddActivityToOutbox(ctx, create, username); err != nil {
			log.Errorf("failed to add activities to datastore: got err=%v", err)
			http.Error(w, fmt.Sprintf("failed to post activity"), http.StatusInternalServerError)
			return
		}
		followers, err := s.Client.FetchFollowers(ctx, identifier)
		if err != nil {
			log.Errorf("failed to fetch followers: got err=%v", err)
			http.Error(w, fmt.Sprintf("failed to post messages"), http.StatusInternalServerError)
			return
		}
		var inboxes []*url.URL
		for _, follower := range followers {
			switch inbox := follower.GetActivityStreamsInbox(); {
			case inbox == nil:
				log.Infof("Inbox of Actor=%q is unset: got %v", follower.GetJSONLDId().Get(), inbox)
				continue
			case inbox.IsIRI():
				log.Infof("Appending %q to list of inboxes", inbox.GetIRI().String())
				inboxes = append(inboxes, inbox.GetIRI())
			default:
				log.Infof("Unexpected value in inbox: got=%v", inbox)
			}
		}
		privateKey, err := s.readPrivateKey(username)
		if err != nil {
			log.Errorf("failed to read private key: got err=%v", err)
			http.Error(w, fmt.Sprintf("failed to send follow request"), http.StatusInternalServerError)
			return
		}
		if person.GetW3IDSecurityV1PublicKey().Empty() {
			log.Errorf("failed to get public key ID: got=%v", person.GetW3IDSecurityV1PublicKey())
			http.Error(w, fmt.Sprintf("failed to send follow request"), http.StatusInternalServerError)
			return
		}
		publicKeyID := person.GetW3IDSecurityV1PublicKey().Begin().Get().GetJSONLDId().Get().String()
		for _, inbox := range inboxes {
			log.Infof("Posting Create Activity to Inbox=%q", inbox.String())
			if err := s.Client.PostToInbox(ctx, inbox, create, publicKeyID, privateKey); err != nil {
				log.Errorf("failed to post to inbox=%q: got err=%v", inbox.String(), err)
			}
		}
	case "Image":
		http.Error(w, fmt.Sprintf("support for Image is unimplemented"), http.StatusNotImplemented)
		return
	default:
		http.Error(w, fmt.Sprintf("unsupported type received"), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) receiveToActorInbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := chi.URLParam(r, "username")
	if username == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
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
	if err != nil {
		log.Errorf("Failed to convert JSON to a vocab.Type: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to unmarshal activity"), http.StatusBadRequest)
		return
	}
	log.Infof("Determining Activity Type: got=%q", activityRequest.GetTypeName())
	switch typeName := activityRequest.GetTypeName(); typeName {
	case "Create":
		if err := s.handleCreateRequest(ctx, activityRequest, username); err != nil {
			log.Errorf("Failed to handle Create Activity: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to process create activity"), http.StatusInternalServerError)
			return
		}
	case "Announce":
		if err := s.handleAnnounceRequest(ctx, activityRequest, username); err != nil {
			log.Errorf("Failed to handle Create Activity: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to process create activity"), http.StatusInternalServerError)
			return
		}
	case "Follow":
		if err := s.handleFollowRequest(ctx, activityRequest); err != nil {
			log.Errorf("Failed to add follower to user: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to accept follower request"), http.StatusInternalServerError)
			return
		}
	case "Accept":
		if err := s.handleAccept(ctx, activityRequest); err != nil {
			log.Errorf("Failed to handle Accept Activity: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to handle Accept activity"), http.StatusInternalServerError)
			return
		}
	case "Undo":
		if err := s.undo(ctx, activityRequest); err != nil {
			log.Errorf("Failed to undo specified Activity: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to undo activity"), http.StatusInternalServerError)
			return
		}
	case "Delete":
		if err := s.delete(ctx, activityRequest); err != nil {
			log.Errorf("Failed to delete specified object: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to delete object"), http.StatusInternalServerError)
			return
		}
	default:
		log.Errorf("Unsupported Type: got=%q", typeName)
		http.Error(w, "failed to process activityRequest", http.StatusInternalServerError)
		return
	}
	if err := s.Datastore.AddActivityToPublicInbox(r.Context(), activityRequest, primitive.NewObjectID()); err != nil {
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
	actor, err := s.Client.FetchRemoteActor(ctx, actorToFollow)
	if err != nil {
		log.Errorf("Failed to retrieve actor: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to send follow request"), http.StatusInternalServerError)
		return
	}
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

	if err := resolver.Resolve(ctx, actor); err != nil {
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
	actorProperty.AppendActivityStreamsPerson(person)
	followActivity.SetActivityStreamsActor(actorProperty)
	objectProperty := streams.NewActivityStreamsObjectProperty()
	objectProperty.AppendActivityStreamsPerson(personToFollow)
	followActivity.SetActivityStreamsObject(objectProperty)
	if err := s.Datastore.AddActivityToPublicInbox(ctx, followActivity, primitive.NewObjectID()); err != nil {
		log.Errorf("Failed to add Follow Activity to Datastore: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to send follow request"), http.StatusInternalServerError)
		return
	}
	privateKey, err := s.readPrivateKey(username)
	if err != nil {
		log.Errorf("failed to read private key: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to send follow request"), http.StatusInternalServerError)
		return
	}
	if person.GetW3IDSecurityV1PublicKey().Empty() {
		log.Errorf("failed to get public key ID: got=%v", person.GetW3IDSecurityV1PublicKey())
		http.Error(w, fmt.Sprintf("failed to send follow request"), http.StatusInternalServerError)
		return
	}
	publicKeyID := person.GetW3IDSecurityV1PublicKey().Begin().Get().GetJSONLDId()
	if err := s.Client.PostToInbox(ctx, inboxURL, followActivity, publicKeyID.Get().String(), privateKey); err != nil {
		log.Errorf("failed to post to inbox: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to send follow request"), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	return
}

func (s *Server) checkFollowStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, claims, err := jwtauth.FromContext(r.Context())
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		return
	}
	rawUserID := claims["userID"]
	var currentUser string
	switch rawUserID.(type) {
	case string:
		currentUser = rawUserID.(string)
	default:
		log.Errorf("Invalid actor ID presented: got %v", rawUserID)
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
		log.Errorf("Failed to unmarshal follower status: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to unmarshal follower status"), http.StatusInternalServerError)
		return
	}
	rawUserID = m["actorID"]
	var otherUser string
	switch rawUserID.(type) {
	case string:
		otherUser = rawUserID.(string)
	default:
		log.Errorf("Invalid actor ID presented: got %v", rawUserID)
		http.Error(w, fmt.Sprintf("failed to load username"), http.StatusUnauthorized)
		return
	}
	followedID, err := s.Client.ResolveActorIdentifierToID(ctx, otherUser)
	if err != nil || followedID == nil {
		log.Errorf("Invalid actor ID presented: got %v", rawUserID)
		http.Error(w, fmt.Sprintf("failed to resolve actor ID"), http.StatusBadRequest)
		return
	}
	status, err := s.Datastore.GetFollowerStatus(ctx, currentUser, followedID.String())
	if err != nil {
		log.Errorf("Failed to determine follower status: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to determine follower status"), http.StatusInternalServerError)
		return
	}
	m = map[string]interface{}{}
	m["followerStatus"] = status
	res, err := json.Marshal(m)
	if err != nil {
		log.Errorf("Failed to marshal response as JSON: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to determine follower status"), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.Write(res)
}

func (s *Server) handleCreateRequest(ctx context.Context, activityRequest vocab.Type, username string) error {
	log.Infoln("Received Create Activity")
	create, err := activity.ParseCreateActivity(ctx, activityRequest)
	if err != nil {
		return err
	}
	if err := s.Client.Create(ctx, create); err != nil {
		return fmt.Errorf("failed to dereference Create Activity: got err=%v", err)
	}
	isReply := false
	for iter := create.GetActivityStreamsObject().Begin(); iter != nil; iter = iter.Next() {
		switch {
		case iter.IsActivityStreamsNote():
			note := iter.GetActivityStreamsNote()
			content := note.GetActivityStreamsContent()
			for c := content.Begin(); c != nil; c = c.Next() {
				c.SetXMLSchemaString(s.Policy.Sanitize(c.GetXMLSchemaString()))
			}
			if note.GetActivityStreamsInReplyTo().Len() != 0 {
				log.Infof("Replies: %d", note.GetActivityStreamsInReplyTo().Len())
				isReply = true
			}
		default:
			return fmt.Errorf("non-note activity presented")
		}
	}
	return s.Datastore.AddActivityToActorInbox(ctx, activityRequest, username, isReply)
}

func (s *Server) handleAnnounceRequest(ctx context.Context, activityRequest vocab.Type, username string) error {
	log.Infoln("Received Announce Activity")
	announce, err := activity.ParseAnnounceActivity(ctx, activityRequest)
	if err != nil {
		return err
	}
	if err := s.Client.Announce(ctx, announce); err != nil {
		return fmt.Errorf("failed to dereference Announce Activity: got err=%v", err)
	}
	for iter := announce.GetActivityStreamsObject().Begin(); iter != nil; iter = iter.Next() {
		switch {
		case iter.IsActivityStreamsNote():
			note := iter.GetActivityStreamsNote()
			content := note.GetActivityStreamsContent()
			for c := content.Begin(); c != nil; c = c.Next() {
				c.SetXMLSchemaString(s.Policy.Sanitize(c.GetXMLSchemaString()))
			}
		case iter.IsActivityStreamsImage():
			image := iter.GetActivityStreamsImage()
			content := image.GetActivityStreamsName()
			for c := content.Begin(); c != nil; c = c.Next() {
				c.SetXMLSchemaString(s.Policy.Sanitize(c.GetXMLSchemaString()))
			}
		default:
			return fmt.Errorf("non-note activity presented")
		}
	}
	return s.Datastore.AddActivityToActorInbox(ctx, activityRequest, username, false)
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
	if err := s.Datastore.AddActivityToPublicInbox(ctx, accept, primitive.NewObjectID()); err != nil {
		return fmt.Errorf("failed to add activity to collection: got err=%v", err)
	}
	object, err := s.Client.FetchRemoteObject(ctx, followerID, false)
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
	if person.GetW3IDSecurityV1PublicKey().Empty() {
		return fmt.Errorf("Public Key is empty on Actor=%q", person.GetJSONLDId().Get().String())
	}
	publicKeyID := person.GetW3IDSecurityV1PublicKey().Begin().Get().GetJSONLDId()
	if err := s.Client.PostToInbox(ctx, remoteActor.GetActivityStreamsInbox().GetIRI(), accept, publicKeyID.Get().String(), privateKey); err != nil {
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

// Delete can only delete/tombstone objects owned by the sending actor or server.
// See: https://www.w3.org/TR/activitypub/#delete-activity-inbox
func (s *Server) delete(ctx context.Context, activityRequest vocab.Type) error {
	log.Infoln("Received Delete Activity")
	deleteActivity, err := activity.ParseDeleteActivity(ctx, activityRequest)
	if err != nil {
		return fmt.Errorf("failed to parse Delete activity: got err=%v", err)
	}
	object := deleteActivity.GetActivityStreamsObject()
	if object == nil {
		return fmt.Errorf("failed to receive Object in Delete activity body: got %v", object)
	}
	for iter := object.Begin(); iter != nil; iter = iter.Next() {
		switch {
		case iter.IsIRI():
			log.Infof("Failed to delete object=%q", iter.GetIRI().String())
		default:
			log.Infof("Failed to delete object=%v", iter)
		}
	}
	return fmt.Errorf("Delete support is unimplemented")
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
	var objectID *url.URL
	switch iter := presentedObject.Begin(); {
	case iter.IsActivityStreamsFollow():
		objectID = iter.GetActivityStreamsFollow().GetJSONLDId().Get()
	default:
		return fmt.Errorf("failed to receive a follow activity: got=%v", iter)
	}
	if err != nil {
		return fmt.Errorf("failed to receive an Object with an IRI: got=%v", objectID)
	}
	loadedObject, err := s.Datastore.GetActivityByActivityID(ctx, objectID.String())
	if err != nil {
		return fmt.Errorf("failed to load follow Activity=%q: got err=%v", objectID.String(), err)
	}
	follow, err := follower.ParseFollowRequest(ctx, loadedObject)
	if err != nil {
		return fmt.Errorf("failed parse follow Activity: got err=%v", err)
	}
	var followerID *url.URL
	switch iter := follow.GetActivityStreamsActor().Begin(); {
	case iter.IsActivityStreamsPerson():
		followerID = iter.GetActivityStreamsPerson().GetJSONLDId().Get()
	default:
		return fmt.Errorf("non person actor specified in Actor property")
	}
	if followerID == nil {
		return fmt.Errorf("follower ID is unspecified: got=%v", followerID)
	}
	var actorID *url.URL
	switch iter := follow.GetActivityStreamsObject().Begin(); {
	case iter.IsActivityStreamsPerson():
		actorID = iter.GetActivityStreamsPerson().GetJSONLDId().Get()
	default:
		return fmt.Errorf("non person actor specified in Object property")
	}
	if actorID == nil {
		return fmt.Errorf("follower ID is unspecified: got=%v", followerID)
	}
	if err := s.Datastore.AddActorToFollows(ctx, actorID.String(), followerID.String()); err != nil {
		return fmt.Errorf("failed to add Follower=%q to Actor=%q: got err=%v", followerID.String(), actorID.String(), err)
	}
	return nil
}

// Webfinger allow other services to query if a user on this instance.
func (s *Server) Webfinger(w http.ResponseWriter, r *http.Request) {
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
	res := &client.WebfingerResponse{
		Subject: resource,
		Links: []client.WebfingerLink{
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

func (s *Server) createToken(username string, userID string, expirationTime time.Time) (string, error) {
	claims := jwt.MapClaims{}
	claims["username"] = username
	claims["userID"] = userID
	claims["exp"] = expirationTime.Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	if s.Secret == "" {
		return "", fmt.Errorf("failed to provide a secret for JWT signing")
	}
	return token.SignedString([]byte(s.Secret))
}

func (s *Server) generateTokenResponse(username string) ([]byte, error) {
	expirationTime := time.Now().Add(72 * time.Hour)
	token, err := s.createToken(username, fmt.Sprintf("%s/actor/%s", s.URL.String(), username), expirationTime)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: got err=%v", err)
	}
	m := map[string]interface{}{}
	m["jwt"] = token
	m["expires"] = expirationTime.UTC().Format(http.TimeFormat)
	marshalledToken, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JWT: got err=%v", err)
	}
	return marshalledToken, nil
}

func parseJWTActorID(claims map[string]interface{}) (string, error) {
	rawUserID := claims["userID"]
	switch rawUserID.(type) {
	case string:
		return rawUserID.(string), nil
	}
	return "", fmt.Errorf("Invalid actor ID presented: got %v", rawUserID)
}

func parseJWTUsername(claims map[string]interface{}) (string, error) {
	rawUserID := claims["username"]
	switch rawUserID.(type) {
	case string:
		return rawUserID.(string), nil
	}
	return "", fmt.Errorf("Invalid actor name presented: got %v", rawUserID)
}
