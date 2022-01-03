package activitypub

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/httprate"
	"net/http"
	"text/template"
	"time"

	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/FediUni/FediUni/activitypub/user"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	log "github.com/golang/glog"
)

type Datastore interface {
	GetActor(context.Context, string) (*actor.Person, error)
	CreateUser(context.Context, *user.User) error
}

type Server struct {
	URL       string
	Router    *chi.Mux
	Datastore Datastore
}

func NewServer(url string, datastore Datastore) *Server {
	s := &Server{
		URL:       url,
		Datastore: datastore,
	}
	s.Router = chi.NewRouter()

	s.Router.Use(middleware.Logger)
	s.Router.Use(middleware.Timeout(60 * time.Second))
	s.Router.Use(httprate.LimitAll(100, time.Minute*1))

	s.Router.Get("/", s.homepage)
	s.Router.Get("/actor/{actorID}", s.getActor)
	s.Router.Get("/actor/{actorID}/inbox", s.getActorInbox)
	s.Router.Get("/actor/{actorID}/outbox", s.getActorOutbox)
	s.Router.Post("/register", s.createUser)
	return s
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
	person, err := actor.NewPerson(username, displayName, s.URL)
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
