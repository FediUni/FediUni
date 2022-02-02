package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"

	"github.com/FediUni/FediUni/activitypub"
	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/FediUni/FediUni/activitypub/mongowrapper"
	log "github.com/golang/glog"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	config = flag.String("config", "/run/secrets/", "The directory with YAML config containing MongoDB URI. This is in addition to the working directory.")
	port   = flag.Int("port", 8080, "The port for the FediUni instance to listen on.")
)

func main() {
	flag.Parse()
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath(*config)
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("unable to load config: got err=%v", err)
	}
	mongoURI := viper.GetString("MONGO_URI")
	if mongoURI == "" {
		log.Fatalf("invalid configuration: MONGO_URI is unspecified")
	}
	instanceURL := viper.GetString("FEDIUNI_URL")
	if instanceURL == "" {
		log.Fatalf("invalid configuration: FEDIUNI_URL is unspecified")
	}
	secret := viper.GetString("SECRET")
	if secret == "" {
		log.Fatalf("failed to receive SECRET")
	}
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			log.Fatalf("failed to disconnect MongoDB client: %v", err)
		}
	}()
	datastore, err := mongowrapper.NewDatastore(client)
	if err != nil {
		log.Fatalln(err)
	}

	s, err := activitypub.NewServer(instanceURL, datastore, actor.NewRSAKeyGenerator(), viper.GetString("SECRET"))
	if err != nil {
		log.Fatalf("failed to create service: got err=%v", err)
	}
	log.Infof("FediUni Instance: Listening on port %d", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), s.Router); err != nil {
		log.Fatalln(err)
	}
}
