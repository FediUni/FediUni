package mongowrapper

import (
	"context"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/FediUni/FediUni/activitypub/user"
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
func (d *Datastore) GetActor(_ context.Context, id string) (*actor.Person, error) {
	return nil, fmt.Errorf("failed to retrieve actor by ID=%s: unimplemented", id)
}

func (d *Datastore) CreatePerson(_ context.Context, actor *actor.Person) error {
	return fmt.Errorf("failed to write actor with ID=%s: unimplemented", actor.Id)
}

func (d *Datastore) CreateUser(_ context.Context, user *user.User) error {
	return fmt.Errorf("failed to write user with Username=%s: unimplemented", user.Username)
}
