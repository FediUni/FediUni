package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/config"
	"github.com/FediUni/FediUni/activitypub/mongowrapper"
	"net/http"
	"os"

	"github.com/FediUni/FediUni/activitypub"

	log "github.com/golang/glog"
)

var (
	port = flag.Int("port", 8080, "The port for the FediUni instance to listen on")
)

func main() {
	mongoURI := os.Getenv("MONGO_URI")
	instanceConfig := config.New()

	ctx := context.Background()
	datastore, err := mongowrapper.NewDatastore(ctx, mongoURI)
	if err != nil {
		log.Fatalln(err)
	}

	s := activitypub.NewServer(instanceConfig, datastore)
	log.Infof("FediUni Instance: %s listening on port %d", instanceConfig.URL, *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), s.Router); err != nil {
		log.Fatalln(err)
	}
}
