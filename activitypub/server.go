package activitypub

import (
	"context"
	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/FediUni/FediUni/activitypub/config"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Datastore interface {
	GetActor(context.Context, string) (*actor.Person, error)
	CreateActor(context.Context, *actor.Person) error
}

type Server struct {
	Config    *config.Config
	Router    *chi.Mux
	Datastore Datastore
}

func NewServer(config *config.Config, datastore Datastore) *Server {
	s := &Server{
		Config:    config,
		Datastore: datastore,
	}
	s.Router = chi.NewRouter()
	s.Router.Use(middleware.Logger)
	s.Router.Get("/actor/{actorID}", s.getActor)
	s.Router.Get("/actor/{actorID}/inbox", s.getActorInbox)
	s.Router.Get("/actor/{actorID}/outbox", s.getActorOutbox)
	return s
}

func (s *Server) getActor(w http.ResponseWriter, r *http.Request) {
	actorID := chi.URLParam(r, "actorID")
	if actorID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	http.Error(w, "actor lookup is unimplemented", http.StatusNotImplemented)
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
