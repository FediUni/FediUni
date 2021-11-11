package activitypub

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"net/http"
)

type Server struct {
	Router *chi.Mux
}

func NewServer() *Server {
	return &Server{
		Router: newRouter(),
	}
}

func newRouter() *chi.Mux {
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Get("/actor/{actorID}", getActor)
	router.Get("/actor/{actorID}/inbox", getActorInbox)
	router.Get("/actor/{actorID}/outbox", getActorOutbox)
	return router
}

func getActor(w http.ResponseWriter, r *http.Request) {
	actorID := chi.URLParam(r, "actorID")
	if actorID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	http.Error(w, "actor lookup is unimplemented", http.StatusNotImplemented)
}

func getActorInbox(w http.ResponseWriter, r *http.Request) {
	actorID := chi.URLParam(r, "actorID")
	if actorID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	http.Error(w, "actor inbox lookup is unimplemented", http.StatusNotImplemented)
}

func getActorOutbox(w http.ResponseWriter, r *http.Request) {
	actorID := chi.URLParam(r, "actorID")
	if actorID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	http.Error(w, "actor outbox lookup is unimplemented", http.StatusNotImplemented)
}
