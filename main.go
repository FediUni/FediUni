package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/config"
	"github.com/FediUni/FediUni/activitypub/mongowrapper"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	client, err := mongo.Connect(ctx, options.Client(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)
	datastore, err := mongowrapper.NewDatastore(client)
	if err != nil {
		log.Fatalln(err)
	}

	s := activitypub.NewServer(instanceConfig, datastore)
	log.Infof("FediUni Instance: %s listening on port %d", instanceConfig.URL, *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), s.Router); err != nil {
		log.Fatalln(err)
	}
}
