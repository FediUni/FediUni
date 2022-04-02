package server

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"github.com/FediUni/FediUni/server/file"
	"github.com/FediUni/FediUni/server/object"
	"github.com/google/go-cmp/cmp"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FediUni/FediUni/server/activity"
	"github.com/FediUni/FediUni/server/actor"
	"github.com/FediUni/FediUni/server/client"
	"github.com/FediUni/FediUni/server/follower"
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
	GetActorByUsername(context.Context, string) (vocab.ActivityStreamsPerson, error)
	GetActivityByObjectID(context.Context, string, string) (vocab.Type, error)
	GetActivityByActivityID(context.Context, string) (vocab.Type, error)
	GetFollowersByUsername(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error)
	GetFollowingByUsername(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error)
	GetFollowerStatus(context.Context, string, string) (int, error)
	CreateUser(context.Context, *user.User) error
	GetUserByUsername(context.Context, string) (*user.User, error)
	AddFollowerToActor(context.Context, string, string) error
	AddActorToFollows(context.Context, string, string) error
	RemoveFollowerFromActor(context.Context, string, string) error
	GetActorByActorID(context.Context, string) (actor.Person, error)
	AddActivityToActivities(context.Context, vocab.Type, primitive.ObjectID) error
	AddObjectsToActorInbox(context.Context, []vocab.Type, string) error
	AddActivityToActorInbox(context.Context, vocab.Type, string, *url.URL) error
	GetActorInbox(context.Context, string, string, string, bool) (vocab.ActivityStreamsOrderedCollectionPage, error)
	GetActorInboxAsOrderedCollection(context.Context, string, bool) (vocab.ActivityStreamsOrderedCollection, error)
	AddActivityToOutbox(context.Context, vocab.Type, string) error
	GetActorOutbox(context.Context, string, string, string) (vocab.ActivityStreamsOrderedCollectionPage, error)
	GetActorOutboxAsOrderedCollection(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error)
	AddActivityToPublicInbox(context.Context, vocab.Type, primitive.ObjectID, bool) error
	GetPublicInbox(context.Context, string, string, bool, bool) (vocab.ActivityStreamsOrderedCollectionPage, error)
	GetPublicInboxAsOrderedCollection(context.Context, bool, bool) (vocab.ActivityStreamsOrderedCollection, error)
	GetLikedAsOrderedCollection(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error)
	GetLikesUsingObjectID(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error)
	GetLikesAsOrderedCollection(context.Context, *url.URL) (vocab.ActivityStreamsOrderedCollection, error)
	DeleteObjectFromAllInboxes(context.Context, *url.URL) error
	AddHostToSameInstitute(ctx context.Context, instance *url.URL) error
	UpdateActor(context.Context, string, string, string, vocab.ActivityStreamsImage) error
	LikeObject(context.Context, *url.URL, *url.URL, *url.URL) error
	GetLikeStatus(context.Context, *url.URL, *url.URL) (bool, error)
	GetAnnounceStatus(context.Context, *url.URL, *url.URL) (bool, error)
}

type Client interface {
	FetchRemoteObject(context.Context, *url.URL, bool, int, int) (vocab.Type, error)
	FetchRemoteActor(ctx context.Context, s string) (actor.Actor, error)
	DereferenceFollowers(ctx context.Context, property vocab.ActivityStreamsFollowersProperty, i int, i2 int) error
	DereferenceFollowing(ctx context.Context, property vocab.ActivityStreamsFollowingProperty, i int, i2 int) error
	DereferenceOutbox(ctx context.Context, property vocab.ActivityStreamsOutboxProperty, i int, i2 int) error
	DereferenceObjectsInOrderedCollection(ctx context.Context, collection vocab.ActivityStreamsOrderedCollection, i int, i2 int, i3 int) (vocab.ActivityStreamsOrderedCollectionPage, error)
	DereferenceOrderedItems(ctx context.Context, property vocab.ActivityStreamsOrderedItemsProperty, i int, i2 int) error
	DereferenceItem(context.Context, vocab.Type, int, int) (vocab.Type, error)
	FetchRemotePerson(ctx context.Context, identifier string) (vocab.ActivityStreamsPerson, error)
	DereferenceRecipientInboxes(ctx context.Context, deliver activity.Activity) ([]*url.URL, error)
	PostToInbox(ctx context.Context, inbox *url.URL, deliver vocab.Type, id string, key *rsa.PrivateKey) error
	LookupInstanceDetails(ctx context.Context, id *url.URL) (*client.InstanceDetails, error)
	ResolveActorIdentifierToID(ctx context.Context, otherUser string) (*url.URL, error)
	Create(ctx context.Context, create vocab.ActivityStreamsCreate, i int, i2 int) error
	Announce(ctx context.Context, announce vocab.ActivityStreamsAnnounce, i int, i2 int) error
}

type Server struct {
	URL          *url.URL
	Router       *chi.Mux
	Actor        *actor.Server
	FileHandler  *file.Handler
	Datastore    Datastore
	Redis        *redis.Client
	KeyGenerator actor.KeyGenerator
	Client       Client
	Policy       *bluemonday.Policy
	Secret       string
}

var (
	tokenAuth *jwtauth.JWTAuth
)

func New(instanceURL *url.URL, datastore Datastore, client Client, keyGenerator actor.KeyGenerator, secret, redisAddress string) (*Server, error) {
	tokenAuth = jwtauth.New("HS256", []byte(secret), nil)
	fileHandler, err := file.NewHandler(viper.GetString("IMAGES_ROOT"))
	if err != nil {
		return nil, err
	}
	s := &Server{
		URL:          instanceURL,
		Actor:        actor.NewServer(instanceURL, datastore, client, fileHandler),
		FileHandler:  fileHandler,
		Datastore:    datastore,
		KeyGenerator: keyGenerator,
		Client:       client,
		Redis: redis.NewClient(&redis.Options{
			Addr:     redisAddress,
			Password: viper.GetString("REDIS_PASSWORD"),
		}),
		Policy: bluemonday.UGCPolicy(),
		Secret: secret,
	}

	s.Router = chi.NewRouter()
	s.Router.Use(middleware.RealIP)
	s.Router.Use(middleware.Logger)
	s.Router.Use(middleware.Timeout(60 * time.Second))
	s.Router.Use(httprate.LimitByIP(300, time.Minute*1))
	s.Router.Use(cors.Handler(cors.Options{
		AllowOriginFunc:  func(r *http.Request, origin string) bool { return true },
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	activitypubRouter := chi.NewRouter()

	images := fmt.Sprintf("%s/", viper.GetString("IMAGES_ROOT"))
	if _, err := os.Stat(images); os.IsNotExist(err) {
		return nil, fmt.Errorf("directory=%q Not Found", images)
	}
	fs := http.StripPrefix("/api/static", http.FileServer(http.Dir(images)))
	activitypubRouter.Handle("/static/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file := strings.Replace(r.RequestURI, "/api/static/", "/", 1)
		log.Infof("File=%q", file)
		if _, err := os.Stat(filepath.Join(images, file)); os.IsNotExist(err) {
			http.Error(w, fmt.Sprintf("Failed to load file"), http.StatusNotFound)
			return
		}
		fs.ServeHTTP(w, r)
	}))

	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Get("/actor", s.getAnyActor)
	activitypubRouter.Get("/actor/{username}", s.getActor)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Get("/actor/{username}/inbox", s.getActorInbox)
	activitypubRouter.With(validation.Signature).Post("/actor/{username}/inbox", s.receiveToActorInbox)
	activitypubRouter.Get("/actor/{username}/outbox", s.getActorOutbox)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Post("/actor/{username}/outbox", s.postActorOutbox)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Post("/actor/{username}/update", s.updateActor)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Get("/actor/outbox", s.getAnyActorOutbox)
	activitypubRouter.Get("/actor/{username}/followers", s.getFollowers)
	activitypubRouter.Get("/actor/{username}/following", s.getFollowing)
	activitypubRouter.Get("/actor/{username}/liked", s.getLiked)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Get("/inbox", s.getPublicInbox)

	activitypubRouter.Post("/register", s.createUser)
	activitypubRouter.Post("/login", s.login)

	activitypubRouter.Get("/activity/{activityID}", s.getActivity)
	activitypubRouter.Get("/activity/{activityID}/object", s.getActivityObject)
	activitypubRouter.Get("/activity/{activityID}/likes", s.getActivityLikes)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Post("/activity/announce/status", s.checkAnnounceStatus)
	activitypubRouter.Get("/activity/likes", s.getAnyLikes)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Post("/activity/likes/status", s.checkLikeStatus)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Get("/activity", s.getAnyActivity)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Post("/follow", s.sendFollowRequest)
	activitypubRouter.With(jwtauth.Verifier(tokenAuth)).Post("/follow/status", s.checkFollowStatus)
	activitypubRouter.Get("/instance", s.getInstanceInfo)

	s.Router.Get("/.well-known/webfinger", s.Webfinger)
	s.Router.Mount("/api", activitypubRouter)
	return s, nil
}

// getActor returns the ActivityPub actor with a corresponding username.
// If the statistics query parameter is true then the number of Followers,
// Actors Followed, and size of Outbox is returned.
func (s *Server) getActor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := chi.URLParam(r, "username")
	if username == "" {
		http.Error(w, "username is unspecified", http.StatusBadRequest)
		return
	}
	statistics := strings.ToLower(r.URL.Query().Get("statistics"))
	actor, err := s.Actor.GetLocalPerson(ctx, username, statistics == "true")
	if err != nil {
		log.Errorf("failed to fetch local actor with username=%q: got err=%v", username, err)
		http.Error(w, "Failed to load local actor", http.StatusNotFound)
		return
	}
	m, err := activity.JSON(actor)
	if err != nil {
		log.Errorf("failed to serialize actor with ID=%q: got err=%v", username, err)
		http.Error(w, "failed to load local actor", http.StatusNotFound)
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

// updateActor allows an actor to update their mutable details.
// This includes their name, summary, and profile picture.
func (s *Server) updateActor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, claims, err := jwtauth.FromContext(ctx)
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to receive JWT"), http.StatusUnauthorized)
		return
	}
	authorizedUsername, err := parseJWTUsername(claims)
	if err != nil {
		log.Errorf("Failed to load username from JWT: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to parse JWT"), http.StatusUnauthorized)
		return
	}
	username := chi.URLParam(r, "username")
	if username == "" {
		http.Error(w, "Username is unspecified", http.StatusBadRequest)
		return
	}
	if authorizedUsername != username {
		http.Error(w, "Not authorized to update Actor", http.StatusUnauthorized)
		return
	}
	// Set MaxMemory to 8MB.
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		log.Errorf("failed to parse Update Form: got err=%v", err)
		http.Error(w, fmt.Sprint("Failed to update user details"), http.StatusBadRequest)
		return
	}
	displayName := r.FormValue("displayName")
	if displayName != "" {
		displayName = s.Policy.Sanitize(displayName)
	}
	summary := r.FormValue("summary")
	if summary != "" {
		summary = s.Policy.Sanitize(summary)
	}
	tempFile, tempFileHeader, err := r.FormFile("profilePicture")
	if err == http.ErrMissingFile {
		log.Infoln("No profile picture presented")
	} else if err != nil {
		log.Errorf("failed to parse profile picture: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to parse profile picture"), http.StatusBadRequest)
		return
	}
	if err := s.Actor.UpdateLocal(ctx, username, displayName, summary, tempFile, tempFileHeader); err != nil {
		log.Errorf("failed to update local actor: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to update Actor details"), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	return
}

// getAnyActor fetches any remote actor based on the identifier parameter.
// The identifier is expected to be of the format @username@domain.
// Webfinger lookup is performed on the domain for the specified actor.
// If the statistics parameter is specified then additional details are loaded.
func (s *Server) getAnyActor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, _, err := jwtauth.FromContext(ctx)
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to receive JWT"), http.StatusUnauthorized)
		return
	}
	identifier := r.URL.Query().Get("identifier")
	if identifier == "" {
		log.Errorf("failed to receive identifier: got=%q", identifier)
		http.Error(w, "Failed to receive an actor identifier", http.StatusBadRequest)
		return
	}
	statistics := r.URL.Query().Get("statistics")
	a, err := s.Actor.GetAny(ctx, identifier, statistics == "true")
	if err != nil {
		log.Errorf("failed to load actor: got err=%v", err)
		http.Error(w, "Failed to find actor", http.StatusNotFound)
		return
	}
	m, err := activity.JSON(a)
	if err != nil {
		log.Errorf("failed to serialize actor: got err=%v", err)
		http.Error(w, "Failed to load actor", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.Write(m)
}

func (s *Server) getAnyActorOutbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, _, err := jwtauth.FromContext(ctx)
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to receive JWT"), http.StatusUnauthorized)
		return
	}
	identifier := r.URL.Query().Get("identifier")
	if identifier == "" {
		log.Errorf("failed to receive identifier: got=%q", identifier)
		http.Error(w, "Failed to receive an Actor identifier", http.StatusBadRequest)
		return
	}
	page, err := s.Actor.GetAnyOutbox(ctx, identifier, 0)
	if err != nil {
		log.Errorf("failed to get Actor=%q Outbox: got err=%v", identifier, err)
		http.Error(w, "Failed to load Outbox", http.StatusNotFound)
		return
	}
	m, err := activity.JSON(page)
	if err != nil {
		log.Errorf("failed to serialize %v: got err=%v", page, err)
		http.Error(w, "Failed to load Outbox", http.StatusNotFound)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.Write(m)
}

func (s *Server) getFollowers(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if username == "" {
		http.Error(w, "username is unspecified", http.StatusBadRequest)
		return
	}
	followers, err := s.Actor.GetFollowers(r.Context(), username)
	if err != nil {
		log.Errorf("failed to load followers from Datastore: got err=%v", err)
		http.Error(w, "Failed to load followers", http.StatusInternalServerError)
		return
	}
	m, err := activity.JSON(followers)
	if err != nil {
		log.Errorf("failed to serialize activity : got err=%v", err)
		http.Error(w, "Failed to load followers", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

func (s *Server) getFollowing(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if username == "" {
		http.Error(w, "username is unspecified", http.StatusBadRequest)
		return
	}
	followers, err := s.Actor.GetFollowing(r.Context(), username)
	if err != nil {
		log.Errorf("failed to load followers from Datastore: got err=%v", err)
		http.Error(w, "Failed to load followers", http.StatusInternalServerError)
		return
	}
	m, err := activity.JSON(followers)
	if err != nil {
		log.Errorf("failed to serialize activity : got err=%v", err)
		http.Error(w, "Failed to load followers", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

func (s *Server) getLiked(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if username == "" {
		http.Error(w, "username is unspecified", http.StatusBadRequest)
		return
	}
	liked, err := s.Actor.GetLikedAsOrderedCollection(r.Context(), username)
	if err != nil {
		log.Errorf("failed to load followers from Datastore: got err=%v", err)
		http.Error(w, "Failed to load followers", http.StatusInternalServerError)
		return
	}
	m, err := activity.JSON(liked)
	if err != nil {
		log.Errorf("failed to serialize activity : got err=%v", err)
		http.Error(w, "Failed to load followers", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

func (s *Server) getActivityLikes(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "activityID")
	if activityID == "" {
		http.Error(w, "Activity ID is unspecified", http.StatusBadRequest)
		return
	}
	liked, err := s.Datastore.GetLikesUsingObjectID(r.Context(), activityID)
	if err != nil {
		log.Errorf("failed to load followers from Datastore: got err=%v", err)
		http.Error(w, "Failed to load followers", http.StatusInternalServerError)
		return
	}
	m, err := activity.JSON(liked)
	if err != nil {
		log.Errorf("failed to serialize activity : got err=%v", err)
		http.Error(w, "Failed to load followers", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

func (s *Server) getActorInbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := chi.URLParam(r, "username")
	if username == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_, claims, err := jwtauth.FromContext(ctx)
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to receive JWT"), http.StatusUnauthorized)
		return
	}
	jwtUsername, err := parseJWTUsername(claims)
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to receive JWT"), http.StatusUnauthorized)
		return
	}
	if username != jwtUsername {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to receive JWT"), http.StatusUnauthorized)
		return
	}
	page := strings.ToLower(r.URL.Query().Get("page")) == "true"
	local := strings.ToLower(r.URL.Query().Get("local")) == "true"
	maxID := r.URL.Query().Get("max_id")
	if maxID == "" {
		maxID = "0"
	}
	minID := r.URL.Query().Get("min_id")
	if minID == "" {
		minID = "0"
	}
	var inbox vocab.Type
	if !page {
		log.Infoln("Getting Inbox as OrderedCollection")
		inbox, err = s.Actor.GetInboxAsOrderedCollection(ctx, username, local)
		if err != nil {
			log.Errorf("Failed to read from Inbox of Username=%q: got err=%v", username, err)
			http.Error(w, fmt.Sprintf("failed to load actor inbox"), http.StatusInternalServerError)
			return
		}
	} else {
		log.Infoln("Getting Inbox as OrderedCollectionPage")
		inbox, err = s.Actor.GetInboxPage(ctx, username, minID, maxID, local)
		if err != nil {
			log.Errorf("Failed to read from Inbox of Username=%q: got err=%v", username, err)
			http.Error(w, fmt.Sprintf("failed to load actor inbox"), http.StatusInternalServerError)
			return
		}
	}
	m, err := activity.JSON(inbox)
	if err != nil {
		log.Errorf("Failed to serialize Inbox of Username=%q: got err=%v", username, err)
		http.Error(w, fmt.Sprintf("Failed to load Actor Inbox"), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

func (s *Server) getPublicInbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, _, err := jwtauth.FromContext(ctx)
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to receive JWT"), http.StatusUnauthorized)
		return
	}
	page := strings.ToLower(r.URL.Query().Get("page")) == "true"
	local := strings.ToLower(r.URL.Query().Get("local")) == "true"
	institute := strings.ToLower(r.URL.Query().Get("institute")) == "true"
	maxID := r.URL.Query().Get("max_id")
	if maxID == "" {
		maxID = "0"
	}
	minID := r.URL.Query().Get("min_id")
	if minID == "" {
		minID = "0"
	}
	var inbox vocab.Type
	if !page {
		inbox, err = s.Actor.GetPublicInboxAsOrderedCollection(ctx, local, institute)
		if err != nil {
			log.Errorf("Failed to read from Public Inbox: got err=%v", err)
			http.Error(w, fmt.Sprintf("failed to load public inbox"), http.StatusInternalServerError)
			return
		}
	} else {
		inbox, err = s.Actor.GetPublicInboxPage(ctx, minID, maxID, local, institute)
		if err != nil {
			log.Errorf("Failed to read from Public Inbox: got err=%v", err)
			http.Error(w, fmt.Sprintf("failed to load public inbox"), http.StatusInternalServerError)
			return
		}
	}
	m, err := activity.JSON(inbox)
	if err != nil {
		log.Errorf("Failed to serialize Public Inbox: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to load Public Inbox"), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

func (s *Server) getActivity(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "activityID")
	if activityID == "" {
		http.Error(w, "activityID is unspecified", http.StatusBadRequest)
	}
	a, err := s.Datastore.GetActivityByObjectID(r.Context(), activityID, s.URL.String())
	if err != nil {
		log.Errorf("failed to get activity with ID=%q: got err=%v", activityID, err)
		http.Error(w, "failed to load activity", http.StatusNotFound)
		return
	}
	m, err := activity.JSON(a)
	if err != nil {
		log.Errorf("failed to serialize activity with ID=%q: got err=%v", activityID, err)
		http.Error(w, "failed to load activity", http.StatusNotFound)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

func (s *Server) getActivityObject(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "activityID")
	if activityID == "" {
		http.Error(w, "activityID is unspecified", http.StatusBadRequest)
	}
	a, err := s.Datastore.GetActivityByObjectID(r.Context(), activityID, s.URL.String())
	if err != nil {
		log.Errorf("failed to get activity with ID=%q: got err=%v", activityID, err)
		http.Error(w, "failed to load activity", http.StatusNotFound)
		return
	}
	m, err := activity.JSON(a)
	if err != nil {
		log.Errorf("failed to serialize activity with ID=%q: got err=%v", activityID, err)
		http.Error(w, "failed to load activity", http.StatusNotFound)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
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
	page := strings.ToLower(r.URL.Query().Get("page")) == "true"
	maxID := r.URL.Query().Get("max_id")
	if maxID == "" {
		maxID = "0"
	}
	minID := r.URL.Query().Get("min_id")
	if minID == "" {
		minID = "0"
	}
	var outbox vocab.Type
	var err error
	if !page {
		outbox, err = s.Actor.GetOutbox(ctx, username)
		if err != nil {
			log.Errorf("Failed to read from Outbox of Actor ID=%q: got err=%v", username, err)
			http.Error(w, fmt.Sprintf("Failed to load Actor inbox"), http.StatusInternalServerError)
			return
		}
	} else {
		outbox, err = s.Actor.GetOutboxPage(ctx, username, minID, maxID)
		if err != nil {
			log.Errorf("Failed to read from Outbox of Actor ID=%q: got err=%v", username, err)
			http.Error(w, fmt.Sprintf("Failed to load Actor inbox"), http.StatusInternalServerError)
			return
		}
	}
	m, err := activity.JSON(outbox)
	if err != nil {
		log.Errorf("Failed to serialize Outbox of Actor ID=%q: got err=%v", username, err)
		http.Error(w, fmt.Sprintf("Failed to load Actor Outbox"), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Set MaxMemory to 8MB.
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		log.Errorf("failed to parse Registration Form: got err=%v", err)
		http.Error(w, fmt.Sprint("failed to create user"), http.StatusBadRequest)
		return
	}
	username := r.FormValue("username")
	if username == "" {
		log.Errorf("failed to create user: username is unspecified")
		http.Error(w, "username is unspecified", http.StatusBadRequest)
	}
	username = strings.ToLower(username)
	displayName := r.FormValue("name")
	password := r.FormValue("password")
	if password == "" {
		log.Errorf("failed to create user: password is unspecified")
		http.Error(w, "password is unspecified", http.StatusBadRequest)
	}
	generator := actor.NewPersonGenerator(s.URL, s.KeyGenerator)
	person, err := generator.NewPerson(ctx, username, displayName)
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
	if err := s.Datastore.CreateUser(ctx, newUser); err != nil {
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
	w.WriteHeader(http.StatusOK)
	w.Write(res)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	// Set MaxMemory to 8 MB.
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		log.Errorf("failed to parse Login Form: got err=%v", err)
		http.Error(w, fmt.Sprint("failed to parse login form"), http.StatusBadRequest)
		return
	}
	username := r.FormValue("username")
	if username == "" {
		log.Errorf("failed to parse Login Form: got username=%q", username)
		http.Error(w, fmt.Sprint("Username is a required field"), http.StatusBadRequest)
		return
	}
	password := r.FormValue("password")
	if password == "" {
		log.Errorf("failed to parse Login Form: got password=%q", password)
		http.Error(w, fmt.Sprint("Password is a required field"), http.StatusBadRequest)
		return
	}
	user, err := s.Datastore.GetUserByUsername(r.Context(), username)
	if err != nil {
		log.Errorf("failed to load username=%q details: got err=%v", username, err)
		http.Error(w, fmt.Sprint("User does not exist on this instance"), http.StatusBadRequest)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		log.Errorf("invalid password provided: got err=%v", err)
		http.Error(w, fmt.Sprint("Password is incorrect"), http.StatusBadRequest)
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

// GetAnyActivity fetches the remote Activity and forces an update.
func (s *Server) getAnyActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, _, err := jwtauth.FromContext(ctx)
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to receive JWT"), http.StatusUnauthorized)
		return
	}
	a := r.URL.Query().Get("id")
	if a == "" {
		http.Error(w, "Activity ID is unspecified", http.StatusBadRequest)
		return
	}
	activityID, err := url.Parse(a)
	if err != nil {
		http.Error(w, "Activity ID is not a URL", http.StatusBadRequest)
		return
	}
	maxDepth := 2
	if r.URL.Query().Get("replies") == strings.ToLower("true") {
		log.Infof("Fetching replies on Activity ID=%q", activityID.String())
		maxDepth += 2
	}
	object, err := s.Client.FetchRemoteObject(ctx, activityID, true, 0, maxDepth)
	if err != nil {
		log.Errorf("Failed to retrieve object ID=%q: got err=%v", activityID, err)
		http.Error(w, "Failed to retrieve activity", http.StatusInternalServerError)
		return
	}
	switch object.GetTypeName() {
	case "Create":
	case "Announce":
	default:
		http.Error(w, "Failed to retrieve a Create or Announce activity", http.StatusBadRequest)
		return
	}
	m, err := activity.JSON(object)
	if err != nil {
		http.Error(w, "Failed to retrieve activity", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

// GetAnyActivity fetches the remote Activity and forces an update.
func (s *Server) getAnyLikes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, _, err := jwtauth.FromContext(ctx)
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to receive JWT"), http.StatusUnauthorized)
		return
	}
	a := r.URL.Query().Get("id")
	if a == "" {
		http.Error(w, "Activity ID is unspecified", http.StatusBadRequest)
		return
	}
	activityID, err := url.Parse(a)
	if err != nil {
		http.Error(w, "Activity ID is not a URL", http.StatusBadRequest)
		return
	}
	likes, err := s.Datastore.GetLikesAsOrderedCollection(ctx, activityID)
	if err != nil {
		log.Errorf("Failed to load Likes for Activity: got err=%v", err)
		likes = streams.NewActivityStreamsOrderedCollection()
		numberOfLikes := streams.NewActivityStreamsTotalItemsProperty()
		numberOfLikes.Set(0)
		likes.SetActivityStreamsTotalItems(numberOfLikes)
	}
	m, err := activity.JSON(likes)
	if err != nil {
		http.Error(w, "Failed to retrieve activity", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

// CheckLikeStatus determines if the specified Actor liked the provided Object.
func (s *Server) checkLikeStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, claims, err := jwtauth.FromContext(r.Context())
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		http.Error(w, "Invalid JWT presented", http.StatusUnauthorized)
		return
	}
	userID, err := parseJWTActorID(claims)
	if err != nil {
		log.Errorf("Failed to read from Actor ID from JWT: got err=%v", err)
		http.Error(w, "Invalid JWT presented", http.StatusUnauthorized)
		return
	}
	actorID, err := url.Parse(userID)
	if err != nil {
		log.Errorf("Failed to read from Actor ID from JWT: got err=%v", err)
		http.Error(w, "Invalid JWT presented", http.StatusUnauthorized)
		return
	}
	raw, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Failed to read request body: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to read body"), http.StatusBadRequest)
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		log.Errorf("Failed to unmarshal like status: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to determine Like status"), http.StatusBadRequest)
		return
	}
	rawObjectID := m["objectID"]
	var objectID *url.URL
	switch rawObjectID.(type) {
	case string:
		id := rawObjectID.(string)
		objectID, err = url.Parse(id)
		if err != nil {
			log.Errorf("Failed to parse Object ID for Like status: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to determine Like status"), http.StatusBadRequest)
			return
		}
	default:
		log.Errorf("Invalid Object ID presented: got %v", rawObjectID)
		http.Error(w, fmt.Sprintf("Failed to determine Like status"), http.StatusBadRequest)
		return
	}
	liked, err := s.Actor.GetLikeStatus(ctx, actorID, objectID)
	if err != nil {
		log.Errorf("Failed to determine Like status: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to determine Like status"), http.StatusBadRequest)
		return
	}
	m = map[string]interface{}{}
	m["likeStatus"] = liked
	res, err := json.Marshal(m)
	if err != nil {
		log.Errorf("Failed to marshal response as JSON: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to determine Like status"), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.Write(res)
}

// checkAnnounceStatus determines if the specified Actor announced the provided Object.
func (s *Server) checkAnnounceStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, claims, err := jwtauth.FromContext(r.Context())
	if err != nil {
		log.Errorf("Failed to read from JWT: got err=%v", err)
		http.Error(w, "Invalid JWT presented", http.StatusUnauthorized)
		return
	}
	userID, err := parseJWTActorID(claims)
	if err != nil {
		log.Errorf("Failed to read from Actor ID from JWT: got err=%v", err)
		http.Error(w, "Invalid JWT presented", http.StatusUnauthorized)
		return
	}
	actorID, err := url.Parse(userID)
	if err != nil {
		log.Errorf("Failed to read from Actor ID from JWT: got err=%v", err)
		http.Error(w, "Invalid JWT presented", http.StatusUnauthorized)
		return
	}
	raw, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Failed to read request body: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to read body"), http.StatusBadRequest)
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		log.Errorf("Failed to unmarshal Announce status: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to determine Announce status"), http.StatusBadRequest)
		return
	}
	rawObjectID := m["objectID"]
	var objectID *url.URL
	switch rawObjectID.(type) {
	case string:
		id := rawObjectID.(string)
		objectID, err = url.Parse(id)
		if err != nil {
			log.Errorf("Failed to parse Object ID for Announce status: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to determine Announce status"), http.StatusBadRequest)
			return
		}
	default:
		log.Errorf("Invalid Object ID presented: got %v", rawObjectID)
		http.Error(w, fmt.Sprintf("Failed to determine Announce status"), http.StatusBadRequest)
		return
	}
	liked, err := s.Datastore.GetAnnounceStatus(ctx, actorID, objectID)
	if err != nil {
		log.Errorf("Failed to determine Announce status: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to determine Announce status"), http.StatusBadRequest)
		return
	}
	m = map[string]interface{}{}
	m["announceStatus"] = liked
	res, err := json.Marshal(m)
	if err != nil {
		log.Errorf("Failed to marshal response as JSON: got err=%v", err)
		http.Error(w, fmt.Sprintf("Failed to determine Like status"), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/activity+json")
	w.Write(res)
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
	rawObject, err := streams.ToType(ctx, m)
	if err != nil {
		log.Errorf("failed to convert JSON to ActivityPub type: got err=%v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	identifier := fmt.Sprintf("@%s@%s", username, s.URL.Host)
	person, err := s.Client.FetchRemotePerson(ctx, identifier)
	if err != nil {
		http.Error(w, fmt.Sprintf("actor does not exist"), http.StatusInternalServerError)
		return
	}
	var toDeliver activity.Activity
	var isReply, isPublic bool
	objectID := primitive.NewObjectID()
	id, err := url.Parse(fmt.Sprintf("%s/activity/%s", s.URL.String(), objectID.Hex()))
	if err != nil {
		log.Errorf("failed to add activities to datastore: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to post activity"), http.StatusInternalServerError)
		return
	}
	idProperty := streams.NewJSONLDIdProperty()
	idProperty.Set(id)
	switch typeName := rawObject.GetTypeName(); typeName {
	case "Announce":
		announce, err := activity.ParseAnnounceActivity(ctx, rawObject)
		if err != nil {
			log.Errorf("Failed to parse Announce activity: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to send Announce"), http.StatusBadRequest)
			return
		}
		objectProperty := announce.GetActivityStreamsObject()
		if objectProperty == nil {
			log.Errorf("Failed to receive an Object: got=%v", objectProperty)
			http.Error(w, fmt.Sprintf("Failed to send Announce"), http.StatusBadRequest)
			return
		}
		var announcedObjectID *url.URL
		for iter := objectProperty.Begin(); iter != objectProperty.End(); iter = iter.Next() {
			if iter.IsIRI() {
				announcedObjectID = iter.GetIRI()
			}
		}
		if announcedObjectID == nil {
			log.Errorf("Failed to receive Object ID in Announce Activity: got=%v", announcedObjectID)
			http.Error(w, fmt.Sprintf("Failed to send Announce"), http.StatusBadRequest)
			return
		}
		announce.SetJSONLDId(idProperty)
		actor := streams.NewActivityStreamsActorProperty()
		actor.AppendActivityStreamsPerson(person)
		announce.SetActivityStreamsActor(actor)
		actorID := person.GetJSONLDId()
		if actorID == nil {
			log.Errorf("Failed to receive Actor ID in Like Activity: got=%v", actorID)
			http.Error(w, fmt.Sprintf("Failed to send Like"), http.StatusBadRequest)
			return
		}
		toDeliver = announce
	case "Like":
		like, err := activity.ParseLikeActivity(ctx, rawObject)
		if err != nil {
			log.Errorf("Failed to parse Like activity: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to send Like"), http.StatusBadRequest)
			return
		}
		objectProperty := like.GetActivityStreamsObject()
		if objectProperty == nil {
			log.Errorf("Failed to receive an Object: got=%v", objectProperty)
			http.Error(w, fmt.Sprintf("Failed to send Like"), http.StatusBadRequest)
			return
		}
		var likedObjectID *url.URL
		for iter := objectProperty.Begin(); iter != objectProperty.End(); iter = iter.Next() {
			if iter.IsIRI() {
				likedObjectID = iter.GetIRI()
			}
		}
		if likedObjectID == nil {
			log.Errorf("Failed to receive Object ID in Like Activity: got=%v", likedObjectID)
			http.Error(w, fmt.Sprintf("Failed to send Like"), http.StatusBadRequest)
			return
		}
		like.SetJSONLDId(idProperty)
		actor := streams.NewActivityStreamsActorProperty()
		actor.AppendActivityStreamsPerson(person)
		like.SetActivityStreamsActor(actor)
		actorID := person.GetJSONLDId()
		if actorID == nil {
			log.Errorf("Failed to receive Actor ID in Like Activity: got=%v", actorID)
			http.Error(w, fmt.Sprintf("Failed to send Like"), http.StatusBadRequest)
			return
		}
		if err := s.Datastore.LikeObject(ctx, likedObjectID, actorID.Get(), id); err != nil {
			log.Errorf("Failed to insert Like: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to send Like"), http.StatusBadRequest)
			return
		}
		toDeliver = like
	case "Note":
		note, err := object.ParseNote(ctx, rawObject)
		if err != nil {
			log.Errorf("Failed to parse Note: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to send Note"), http.StatusBadRequest)
			return
		}
		noteID, err := url.Parse(fmt.Sprintf("%s/object", id.String()))
		if err != nil {
			log.Errorf("Failed to create Note ID: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to send Note"), http.StatusBadRequest)
			return
		}
		noteIDProperty := streams.NewJSONLDIdProperty()
		noteIDProperty.Set(noteID)
		note.SetJSONLDId(noteIDProperty)
		for iter := note.GetActivityStreamsAttributedTo().Begin(); iter != nil; iter = iter.Next() {
			if !iter.IsIRI() {
				continue
			}
			if iter.GetIRI().String() != userID {
				http.Error(w, fmt.Sprintf("attributedTo mismatch in note"), http.StatusUnauthorized)
				return
			}
		}
		create, err := object.WrapInCreate(ctx, note, person)
		if err != nil {
			log.Errorf("Failed to wrap Note in Create Activity: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to send Note"), http.StatusBadRequest)
			return
		}
		create.SetJSONLDId(idProperty)
		if inReplyToProperty := note.GetActivityStreamsInReplyTo(); inReplyToProperty != nil {
			for iter := inReplyToProperty.Begin(); iter != inReplyToProperty.End(); iter = iter.Next() {
				if iter.IsIRI() && iter.GetIRI() != nil {
					isReply = true
					break
				}
			}
		}
		for iter := note.GetActivityStreamsTo().Begin(); iter != nil; iter = iter.Next() {
			if iter.GetIRI().String() == "https://www.w3.org/ns/activitystreams#Public" {
				isPublic = true
				break
			}
		}
		toDeliver = create
	case "Event":
		event, err := object.ParseEvent(ctx, rawObject)
		if err != nil {
			log.Errorf("Failed to parse Event: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to send Event"), http.StatusBadRequest)
			return
		}
		eventID, err := url.Parse(fmt.Sprintf("%s/object", id.String()))
		if err != nil {
			log.Errorf("Failed to create Event ID: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to send Event"), http.StatusBadRequest)
			return
		}
		eventIDProperty := streams.NewJSONLDIdProperty()
		eventIDProperty.Set(eventID)
		event.SetJSONLDId(eventIDProperty)
		for iter := event.GetActivityStreamsAttributedTo().Begin(); iter != nil; iter = iter.Next() {
			if !iter.IsIRI() {
				continue
			}
			if iter.GetIRI().String() != userID {
				http.Error(w, fmt.Sprintf("attributedTo mismatch in Event"), http.StatusUnauthorized)
				return
			}
		}
		create, err := object.WrapInCreate(ctx, event, person)
		if err != nil {
			log.Errorf("Failed to wrap Event in Create Activity: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to send Event"), http.StatusBadRequest)
			return
		}
		create.SetJSONLDId(idProperty)
		if inReplyToProperty := event.GetActivityStreamsInReplyTo(); inReplyToProperty != nil {
			for iter := inReplyToProperty.Begin(); iter != inReplyToProperty.End(); iter = iter.Next() {
				if iter.IsIRI() && iter.GetIRI() != nil {
					isReply = true
					break
				}
			}
		}
		for iter := event.GetActivityStreamsTo().Begin(); iter != nil; iter = iter.Next() {
			if iter.GetIRI().String() == "https://www.w3.org/ns/activitystreams#Public" {
				isPublic = true
				break
			}
		}
		toDeliver = create
	case "Image":
		http.Error(w, fmt.Sprintf("support for Image is unimplemented"), http.StatusNotImplemented)
		return
	default:
		http.Error(w, fmt.Sprintf("unsupported type received"), http.StatusInternalServerError)
		return
	}
	if err := s.Datastore.AddActivityToActivities(ctx, toDeliver, objectID); err != nil {
		log.Errorf("failed to add activities to datastore: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to post activity"), http.StatusInternalServerError)
		return
	}
	if err := s.Datastore.AddActivityToOutbox(ctx, toDeliver, username); err != nil {
		log.Errorf("failed to add activities to datastore: got err=%v", err)
		http.Error(w, fmt.Sprintf("failed to post activity"), http.StatusInternalServerError)
		return
	}
	inboxes, err := s.Client.DereferenceRecipientInboxes(ctx, toDeliver)
	// Post to own inbox to allow user to view their own activities.
	if rawObject.GetTypeName() == "Note" {
		inboxes = append(inboxes, person.GetActivityStreamsInbox().GetIRI())
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
		log.Infof("Posting Activity to Inbox=%q", inbox.String())
		if err := s.Client.PostToInbox(ctx, inbox, toDeliver, publicKeyID, privateKey); err != nil {
			log.Errorf("failed to post to inbox=%q: got err=%v", inbox.String(), err)
		}
	}
	if !isPublic {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := s.Datastore.AddActivityToPublicInbox(ctx, toDeliver, primitive.NewObjectID(), isReply); err != nil {
		log.Errorf("failed to add activity to public inbox: got err=%v", err)
		w.WriteHeader(http.StatusInternalServerError)
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
	id := activityRequest.GetJSONLDId().Get()
	details, err := s.Client.LookupInstanceDetails(ctx, id)
	if err != nil {
		log.Errorf("Unable to fetch instance details: got err=%v", err)
	}
	if details == nil {
		log.Infof("Host=%q is not a FediUni instance", id.Host)
	} else if details.Institute != "" && details.Institute == viper.GetString("INSTITUTE_NAME") {
		if err := s.Datastore.AddHostToSameInstitute(ctx, id); err != nil {
			log.Errorf("Failed to add host to known institutes: got err=%v", err)
		}
	}
	dump, err := httputil.DumpRequestOut(r, true)
	if err != nil {
		log.Errorf("Can't dump: %v", err)
	} else {
		log.Infof("%s", dump)
	}
	switch typeName := activityRequest.GetTypeName(); typeName {
	case "Create":
		if err := s.handleCreateRequest(ctx, activityRequest, username); err != nil {
			log.Errorf("Failed to handle Create Activity: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to process create activity"), http.StatusInternalServerError)
			return
		}
	case "Announce":
		if err := s.handleAnnounceRequest(ctx, activityRequest, username); err != nil {
			log.Errorf("Failed to handle Announce Activity: got err=%v", err)
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
	case "Update":
		if err := s.update(ctx, activityRequest); err != nil {
			log.Errorf("Failed to delete specified object: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to delete object"), http.StatusInternalServerError)
			return
		}
	case "Like":
		if err := s.like(ctx, activityRequest); err != nil {
			log.Errorf("Failed to like specified object: got err=%v", err)
			http.Error(w, fmt.Sprintf("Failed to like object"), http.StatusBadRequest)
		}
	default:
		log.Errorf("Unsupported Type: got=%q", typeName)
		http.Error(w, "failed to process activityRequest", http.StatusInternalServerError)
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
	if err := s.Datastore.AddActivityToActivities(ctx, followActivity, primitive.NewObjectID()); err != nil {
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
	if err := s.Client.Create(ctx, create, 0, 2); err != nil {
		return fmt.Errorf("failed to dereference Create Activity: got err=%v", err)
	}
	var inReplyTo *url.URL
	for iter := create.GetActivityStreamsObject().Begin(); iter != nil; iter = iter.Next() {
		switch {
		case iter.IsActivityStreamsNote():
			note := iter.GetActivityStreamsNote()
			content := note.GetActivityStreamsContent()
			for c := content.Begin(); c != content.End(); c = c.Next() {
				c.SetXMLSchemaString(s.Policy.Sanitize(c.GetXMLSchemaString()))
			}
			if inReplyToProperty := note.GetActivityStreamsInReplyTo(); inReplyToProperty != nil {
				for iter := inReplyToProperty.Begin(); iter != inReplyToProperty.End(); iter = iter.Next() {
					if iter.IsIRI() && iter.GetIRI() != nil {
						inReplyTo = iter.GetIRI()
						break
					}
				}
			}
		case iter.IsActivityStreamsEvent():
			event := iter.GetActivityStreamsEvent()
			content := event.GetActivityStreamsContent()
			for c := content.Begin(); c != content.End(); c = c.Next() {
				c.SetXMLSchemaString(s.Policy.Sanitize(c.GetXMLSchemaString()))
			}
		default:
			return fmt.Errorf("non-note activity presented")
		}
	}
	if err := s.Datastore.AddActivityToActorInbox(ctx, activityRequest, username, inReplyTo); err != nil {
		return fmt.Errorf("failed to add to actor inbox: got err=%v", err)
	}
	// Don't forward replies to the public inbox to avoid clutter.
	if inReplyTo != nil {
		return nil
	}
	for iter := create.GetActivityStreamsTo().Begin(); iter != nil; iter = iter.Next() {
		if iter.GetIRI().String() == "https://www.w3.org/ns/activitystreams#Public" {
			log.Infof("Posting ID=%q to Public Inbox", create.GetJSONLDId().Get().String())
			return s.Datastore.AddActivityToPublicInbox(ctx, activityRequest, primitive.NewObjectID(), inReplyTo != nil)
		}
	}
	return nil
}

func (s *Server) handleAnnounceRequest(ctx context.Context, activityRequest vocab.Type, username string) error {
	log.Infoln("Received Announce Activity")
	announce, err := activity.ParseAnnounceActivity(ctx, activityRequest)
	if err != nil {
		return err
	}
	if err := s.Client.Announce(ctx, announce, 0, 2); err != nil {
		return fmt.Errorf("failed to dereference Announce Activity: got err=%v", err)
	}
	for iter := announce.GetActivityStreamsObject().Begin(); iter != nil; iter = iter.Next() {
		switch {
		case iter.IsIRI():
			o, err := s.Client.FetchRemoteObject(ctx, iter.GetIRI(), false, 0, 2)
			if err != nil {
				return err
			}
			if err := iter.SetType(o); err != nil {
				return err
			}
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
		case iter.IsActivityStreamsCreate():
			// Overwrite Create with the first Object.
			create, err := activity.ParseCreateActivity(ctx, iter.GetActivityStreamsCreate())
			if err != nil {
				return err
			}
			o := create.GetActivityStreamsObject()
			for i := o.Begin(); i != o.End(); {
				if err := iter.SetType(i.GetType()); err != nil {
					return err
				}
				break
			}
		default:
			return fmt.Errorf("non-note activity presented: got=%v", iter.GetType())
		}
	}
	return s.Datastore.AddActivityToActorInbox(ctx, activityRequest, username, nil)
}

// handleFollowRequest allows the actor to follow the requested person on this instance.
// Currently, follow automatically accepts all incoming handleFollowRequest requests, but
// users should be able to confirm and deny handleFollowRequest requests before the Accept
// activity is sent.
func (s *Server) handleFollowRequest(ctx context.Context, activityRequest vocab.Type) error {
	log.Infoln("Received Follow Activity")
	follow, err := activity.ParseFollowActivity(ctx, activityRequest)
	if err != nil {
		return fmt.Errorf("failed to parse handleFollowRequest activityRequest: got err=%v", err)
	}
	var followerID *url.URL
	followingActor := follow.GetActivityStreamsActor()
	for iter := followingActor.Begin(); iter != followingActor.End(); iter = iter.Next() {
		switch {
		case iter.IsIRI():
			followerID = iter.GetIRI()
		case iter.IsActivityStreamsPerson():
			person := iter.GetActivityStreamsPerson()
			personIDProperty := person.GetJSONLDId()
			if personIDProperty == nil {
				return fmt.Errorf("failed to handle FollowRequest: got Person ID Property=%v", personIDProperty)
			}
			followerID = person.GetJSONLDId().Get()
		default:
			return fmt.Errorf("unexpected Actor type received: got Actor=%v", followingActor)
		}
	}
	if followerID == nil {
		return fmt.Errorf("follower ID is unspecified: got=%q", followerID)
	}
	var followedID *url.URL
	followedActor := follow.GetActivityStreamsObject()
	for iter := followedActor.Begin(); iter != followedActor.End(); iter = iter.Next() {
		switch {
		case iter.IsIRI():
			followedID = iter.GetIRI()
		case iter.IsActivityStreamsPerson():
			person := iter.GetActivityStreamsPerson()
			personIDProperty := person.GetJSONLDId()
			if personIDProperty == nil {
				return fmt.Errorf("failed to handle FollowRequest: got Person ID Property=%v", personIDProperty)
			}
			followedID = person.GetJSONLDId().Get()
		default:
			return fmt.Errorf("unexpected Object type received: got Object=%v", followingActor)
		}
	}
	if followedID == nil {
		return fmt.Errorf("followed ID is unspecified: got=%q", followerID)
	}
	accept := follower.PrepareAcceptActivity(follow, followedID)
	// Ensure Actor exists on this server before adding activity.
	person, err := s.Datastore.GetActorByActorID(ctx, followedID.String())
	if err != nil {
		return fmt.Errorf("failed to load person: got err=%v", err)
	}
	if err := s.Datastore.AddActivityToActivities(ctx, accept, primitive.NewObjectID()); err != nil {
		return fmt.Errorf("failed to add activity to collection: got err=%v", err)
	}
	object, err := s.Client.FetchRemoteObject(ctx, followerID, false, 0, 2)
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
	if err := s.Datastore.AddFollowerToActor(ctx, followedID.String(), followerID.String()); err != nil {
		return fmt.Errorf("failed to add follower to actor: got err=%v", err)
	}
	return nil
}

// Undo can only reverse the effects of Like, Follow, or Block Activities.
// See: https://www.w3.org/TR/activitypub/#undo-activity-outbox
func (s *Server) undo(ctx context.Context, activityRequest vocab.Type) error {
	log.Infoln("Received Undo Activity")
	undoRequest, err := activity.ParseUndoActivity(ctx, activityRequest)
	if err != nil {
		return fmt.Errorf("failed to parse Undo activity: got err=%v", err)
	}
	object := undoRequest.GetActivityStreamsObject()
	if object == nil {
		return fmt.Errorf("failed to receive Object in Undo activity body: got %v", object)
	}
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
	var objectID *url.URL
	for iter := object.Begin(); iter != nil; iter = iter.Next() {
		switch {
		case iter.IsIRI():
			objectID = iter.GetIRI()
		default:
			o := iter.GetType()
			if o == nil {
				return fmt.Errorf("failed to delete object: object=%v", o)
			}
			objectID = o.GetJSONLDId().Get()
		}
	}
	deleteID := deleteActivity.GetJSONLDId().Get()
	if deleteID == nil {
		return fmt.Errorf("failed to receive an ID in Delete Activity: got err=%v", err)
	}
	if d := cmp.Diff(deleteID.Host, objectID.Host); d != "" {
		return fmt.Errorf("mismatch in ids: (+got -want) %s", d)
	}
	if err := s.Datastore.DeleteObjectFromAllInboxes(ctx, deleteID); err != nil {
		return fmt.Errorf("failed to remove object: got err=%v", err)
	}
	return nil
}

// Update can only update activities and objects owned by the actor or server.
// See: https://www.w3.org/TR/activitypub/#update-activity-inbox
func (s *Server) update(ctx context.Context, activityRequest vocab.Type) error {
	return fmt.Errorf("support for updating activities and objects is unimplemented")
}

// Like can only like activities and objects owned by the server.
// See: https://www.w3.org/TR/activitypub/#like-activity-inbox
func (s *Server) like(ctx context.Context, activityRequest vocab.Type) error {
	like, err := activity.ParseLikeActivity(ctx, activityRequest)
	if err != nil {
		return err
	}
	id := activityRequest.GetJSONLDId()
	if id == nil {
		return fmt.Errorf("failed to receive an ID in Activity: got=%v", id)
	}
	activityID := id.Get()
	actorProperty := like.GetActivityStreamsActor()
	if actorProperty == nil {
		return fmt.Errorf("failed to receive an Actor in Activity: got=%v", actorProperty)
	}
	var actorID *url.URL
	for iter := actorProperty.Begin(); iter != actorProperty.End(); iter = iter.Next() {
		switch {
		case iter.IsIRI():
			actorID = iter.GetIRI()
		case iter.HasAny():
			a := iter.GetType()
			if a == nil {
				return fmt.Errorf("failed to receive Actor: got=%v", a)
			}
			id := a.GetJSONLDId()
			if id == nil {
				return fmt.Errorf("failed to receive Actor ID: got=%v", id)
			}
			actorID = id.Get()
		}
	}
	likedObject := like.GetActivityStreamsObject()
	if likedObject == nil {
		return fmt.Errorf("liked Object is unspecified: got=%v", likedObject)
	}
	var objectID *url.URL
	for iter := likedObject.Begin(); iter != likedObject.End(); iter = iter.Next() {
		switch {
		case iter.IsIRI():
			objectID = iter.GetIRI()
		case iter.HasAny():
			o := iter.GetType()
			id := o.GetJSONLDId()
			if id == nil {
				return fmt.Errorf("liked Object ID is undefined: got=%v", id)
			}
			objectID = id.Get()
		default:
			return fmt.Errorf("failed to receive Object: got=%v", iter)
		}
	}
	if err := s.Datastore.LikeObject(ctx, objectID, actorID, activityID); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleAccept(ctx context.Context, activityRequest vocab.Type) error {
	accept, err := activity.ParseAcceptActivity(ctx, activityRequest)
	if err != nil {
		return err
	}
	presentedObject := accept.GetActivityStreamsObject()
	if presentedObject.Empty() {
		return fmt.Errorf("failed to receive an Object in Accept Activity: got=%v", presentedObject)
	}
	var objectID *url.URL
	switch iter := presentedObject.Begin(); {
	case iter.IsIRI():
		objectID = iter.GetIRI()
	case iter.IsActivityStreamsFollow():
		objectIDProperty := iter.GetActivityStreamsFollow().GetJSONLDId()
		if objectIDProperty == nil {
			return fmt.Errorf("failed to receive a valid Object: got Object without ID: %v", objectIDProperty)
		}
		objectID = objectIDProperty.Get()
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
	follow, err := activity.ParseFollowActivity(ctx, loadedObject)
	if err != nil {
		return fmt.Errorf("failed parse follow Activity: got err=%v", err)
	}
	var followerID *url.URL
	switch iter := follow.GetActivityStreamsActor().Begin(); {
	case iter.IsIRI():
		followerID = iter.GetIRI()
	case iter.IsActivityStreamsPerson():
		followerIDProperty := iter.GetActivityStreamsPerson().GetJSONLDId()
		if followerIDProperty == nil {
			return fmt.Errorf("failed to receive a valid Person: got Person with ID Property=%v", followerIDProperty)
		}
		followerID = followerIDProperty.Get()
	default:
		return fmt.Errorf("non person actor specified in Actor property")
	}
	if followerID == nil {
		return fmt.Errorf("follower ID is unspecified: got=%v", followerID)
	}
	var actorID *url.URL
	switch iter := follow.GetActivityStreamsObject().Begin(); {
	case iter.IsIRI():
		actorIDProperty := iter.GetActivityStreamsPerson().GetJSONLDId()
		if actorIDProperty == nil {
			return fmt.Errorf("failed to receive a valid Person: got Person with ID Property=%v", actorIDProperty)
		}
		actorID = actorIDProperty.Get()
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

func (s *Server) getInstanceInfo(w http.ResponseWriter, r *http.Request) {
	name := viper.GetString("INSTANCE_NAME")
	if name == "" {
		log.Errorf("INSTANCE_NAME is undefined in config")
	}
	institute := viper.GetString("INSTITUTE_NAME")
	if institute == "" {
		log.Errorf("INSTITUTE_NAME is undefined in config")
	}
	details := &client.InstanceDetails{
		Name:      name,
		Institute: institute,
	}
	marshalledDetails, err := json.Marshal(details)
	if err != nil {
		http.Error(w, "Failed to retrieve instance details", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(marshalledDetails)
	w.WriteHeader(http.StatusOK)
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
