package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/FediUni/FediUni/server/client"
	"net/http"
	"net/url"

	"github.com/FediUni/FediUni/server"
	"github.com/FediUni/FediUni/server/actor"
	"github.com/FediUni/FediUni/server/mongowrapper"
	log "github.com/golang/glog"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	config = flag.String("config", "/run/secrets/config.yaml", "The file with config containing MongoDB URI, etc.")
	port   = flag.Int("port", 8080, "The port for the FediUni instance to listen on.")
)

func main() {
	flag.Parse()
	viper.SetConfigFile(*config)
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
	mongoClient, err := mongo.Connect(ctx, options.Client(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}
	defer func() {
		if err := mongoClient.Disconnect(ctx); err != nil {
			log.Fatalf("failed to disconnect MongoDB client: %v", err)
		}
	}()
	url, err := url.Parse(instanceURL)
	if err != nil {
		log.Fatalf("failed to parse instanceURL=%q: got err=%v", instanceURL, err)
	}
	datastore, err := mongowrapper.NewDatastore(mongoClient, "FediUni", url)
	if err != nil {
		log.Fatalln(err)
	}
	redisAddress := "redis:6379"
	httpClient := client.NewClient(url, redisAddress, viper.GetString("REDIS_PASSWORD"), datastore)
	s, err := server.New(url, datastore, httpClient, actor.NewRSAKeyGenerator(), viper.GetString("SECRET"), redisAddress)
	if err != nil {
		log.Fatalf("failed to create service: got err=%v", err)
	}
	log.Infof("FediUni Instance: Listening on port %d", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), s.Router); err != nil {
		log.Fatalln(err)
	}
}
