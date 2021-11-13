package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/config"
	"github.com/FediUni/FediUni/activitypub/mongowrapper"
	"net/http"

	"github.com/FediUni/FediUni/activitypub"

	log "github.com/golang/glog"
)

var (
	mongoURI = flag.String("mongo_uri", "", "This is the URI to be used when connecting to MongoDB")
	port     = flag.Int("port", 8080, "The port for the FediUni instance to listen on")
)

func main() {
	ctx := context.Background()
	instanceConfig := config.New()
	datastore, err := mongowrapper.NewDatastore(ctx, *mongoURI)
	if err != nil {
		log.Fatalln(err)
	}
	s := activitypub.NewServer(instanceConfig, datastore)
	log.Infof("FediUni Instance: %s listening on port %d", config.URL, *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), s.Router); err != nil {
		log.Fatalln(err)
	}
}
