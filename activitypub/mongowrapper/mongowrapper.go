package mongowrapper

import (
	"context"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/FediUni/FediUni/activitypub/user"
	log "github.com/golang/glog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Datastore wraps the MongoDB client and handles MongoDB operations.
type Datastore struct {
	client *mongo.Client
}

// NewDatastore returns an initialized Datastore which handles MongoDB operations.
func NewDatastore(client *mongo.Client) (*Datastore, error) {
	return &Datastore{
		client: client,
	}, nil
}

// GetActor returns an instance of Person from Mongo.
func (d *Datastore) GetActor(ctx context.Context, username string) (*actor.Person, error) {
	users := d.client.Database("FediUni").Collection("users")
	filter := bson.D{{"username", username}}
	user := &user.User{}
	if err := users.FindOne(ctx, filter).Decode(&user); err != nil {
		return nil, err
	}
	if user.Person == nil {
		return nil, fmt.Errorf("unable to load user with username=%q", username)
	}
	return user.Person, nil
}

func (d *Datastore) CreateUser(ctx context.Context, user *user.User) error {
	users := d.client.Database("FediUni").Collection("users")
	marshalledUser, err := user.BSON()
	if err != nil {
		return err
	}
	res, err := users.InsertOne(ctx, marshalledUser)
	if err != nil {
		return err
	}
	log.Infof("Inserted user %q with _id=%q", user.Username, res.InsertedID)
	return nil
}
