package activitypub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	GetActor(context.Context, string) (*actor.Person, error)
	CreateUser(context.Context, *user.User) error
}

type Server struct {
	URL          *url.URL
	Keys         string
	Router       *chi.Mux
	Datastore    Datastore
	KeyGenerator actor.KeyGenerator
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
	s.Router.Get("/actor/{actorID}", s.getActor)
	s.Router.Get("/actor/{actorID}/inbox", s.getActorInbox)
	s.Router.Get("/actor/{actorID}/outbox", s.getActorOutbox)
	s.Router.Post("/register", s.createUser)
	return s, nil
}

func (s *Server) homepage(w http.ResponseWriter, r *http.Request) {
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
	actorID := chi.URLParam(r, "actorID")
	if actorID == "" {
		http.Error(w, "actorID is unspecified", http.StatusBadRequest)
		return
	}
	person, err := s.Datastore.GetActor(r.Context(), actorID)
	if err != nil {
		log.Errorf("failed to get actor with ID=%q: got err=%v", actorID, err)
		http.Error(w, "failed to load actor", http.StatusNotFound)
		return
	}
	marshalledPerson, err := json.Marshal(person)
	if err != nil {
		log.Errorf("failed to get actor with ID=%q: got err=%v", actorID, err)
		http.Error(w, "failed to load actor", http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusOK)
	w.Write(marshalledPerson)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
	}
	username := r.FormValue("username")
	displayName := r.FormValue("displayName")
	password := r.FormValue("password")
	person, err := actor.NewPerson(username, displayName, s.URL.String(), s.KeyGenerator)
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
	actorID := chi.URLParam(r, "actorID")
	if actorID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	http.Error(w, "actor inbox lookup is unimplemented", http.StatusNotImplemented)
}

func (s *Server) getActorOutbox(w http.ResponseWriter, r *http.Request) {
	actorID := chi.URLParam(r, "actorID")
	if actorID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	http.Error(w, "actor outbox lookup is unimplemented", http.StatusNotImplemented)
}

func (s *Server) webfinger(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Query().Get("resource")
	var username string
	// Find alternative to this parsing (other than Regex
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
	actor, err := s.Datastore.GetActor(r.Context(), username)
	if err != nil {
		http.Error(w, fmt.Sprintf("unable to find actor"), http.StatusNotFound)
		return
	}
	res := &WebfingerResponse{
		Subject: resource,
		Links: []WebfingerLink{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: actor.Id,
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
