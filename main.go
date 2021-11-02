package main

import (
	"net/http"

	"github.com/FediUni/FediUni/activitypub"

	log "github.com/golang/glog"
)

func main() {
	s := activitypub.NewServer()

	if err := http.ListenAndServe(":8080", s.Router); err != nil {
		log.Fatalln(err)
	}
}
