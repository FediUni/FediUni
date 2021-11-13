package mongowrapper

import (
	"context"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/actor"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Datastore wraps the MongoDB client and handles MongoDB operations.
type Datastore struct {
	client *mongo.Client
}

// NewDatastore returns an initialized Datastore which handles MongoDB operations.
func NewDatastore(ctx context.Context, uri string) (*Datastore, error) {
	client, err := mongo.Connect(ctx, options.Client(), options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}
	return &Datastore{
		client: client,
	}, nil
}

// GetActor returns an instance of Person from Mongo.
func (d *Datastore) GetActor(_ context.Context, id string) (*actor.Person, error) {
	return nil, fmt.Errorf("failed to retrieve actor by ID=%s: unimplemented", id)
}
