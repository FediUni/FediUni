package mongowrapper

import (
	"context"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/FediUni/FediUni/activitypub/user"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	log "github.com/golang/glog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"net/url"
	"strings"
	"time"
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

// GetActorByUsername returns an instance of Person from Mongo using Username.
func (d *Datastore) GetActorByUsername(ctx context.Context, username string) (actor.Person, error) {
	actors := d.client.Database("FediUni").Collection("actors")
	filter := bson.D{{"preferredUsername", strings.ToLower(username)}}
	var m map[string]interface{}
	if err := actors.FindOne(ctx, filter).Decode(&m); err != nil {
		return nil, err
	}
	var actor actor.Person
	resolver, err := streams.NewJSONResolver(func(ctx context.Context, person vocab.ActivityStreamsPerson) error {
		actor = person
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create a Person resolver: got err=%v", err)
	}
	if err = resolver.Resolve(ctx, m); err != nil {
		return nil, fmt.Errorf("failed to resolve JSON to Person: got err=%v", err)
	}
	return actor, nil
}

// GetActorByActorID returns an instance of Person from Mongo using URI.
func (d *Datastore) GetActorByActorID(ctx context.Context, actorID string) (actor.Person, error) {
	actors := d.client.Database("FediUni").Collection("actors")
	filter := bson.D{{"id", actorID}}
	var m map[string]interface{}
	if err := actors.FindOne(ctx, filter).Decode(&m); err != nil {
		return nil, err
	}
	var actor actor.Person
	resolver, err := streams.NewJSONResolver(ctx, func(ctx context.Context, person vocab.ActivityStreamsPerson) error {
		actor = person
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create a Person resolver: got err=%v", err)
	}
	if err = resolver.Resolve(ctx, m); err != nil {
		return nil, fmt.Errorf("failed to resolve JSON to Person: got err=%v", err)
	}
	return actor, nil
}

func (d *Datastore) CreateUser(ctx context.Context, user *user.User) error {
	users := d.client.Database("FediUni").Collection("users")
	res, err := users.InsertOne(ctx, bson.D{{"username", strings.ToLower(user.Username)}, {"password", user.Password}})
	if err != nil {
		return err
	}
	log.Infof("Inserted user %q with _id=%q", user.Username, res.InsertedID)
	actors := d.client.Database("FediUni").Collection("actors")
	serializedPerson, err := streams.Serialize(user.Person)
	if err != nil {
		return fmt.Errorf("failed to serialize user %q: got err=%v", user.Username, err)
	}
	m, err := bson.Marshal(serializedPerson)
	if err != nil {
		return fmt.Errorf("failed to marshal serialiazed user %q: got err=%v", user.Username, err)
	}
	res, err = actors.InsertOne(ctx, m)
	if err != nil {
		return err
	}
	log.Infof("Inserted actor %q with _id=%q", user.Username, res.InsertedID)
	return nil
}

func (d *Datastore) AddActivityToSharedInbox(ctx context.Context, activity vocab.Type, baseURL string) error {
	activities := d.client.Database("FediUni").Collection("activities")
	objectID := primitive.NewObjectIDFromTimestamp(time.Now())
	id, err := url.Parse(fmt.Sprintf("%s/activity/%s", baseURL, objectID))
	if err != nil {
		return err
	}
	if activity.GetJSONLDId().Get().String() == "" {
		idProperty := streams.NewJSONLDIdProperty()
		idProperty.Set(id)
		activity.SetJSONLDId(idProperty)
	}
	marshalledActivity, err := streams.Serialize(activity)
	if err != nil {
		return err
	}
	res, err := activities.InsertOne(ctx, marshalledActivity)
	if err != nil {
		return err
	}
	log.Infof("Inserted activity %q with _id=%q", id.String(), res.InsertedID)
	return nil
}

func (d *Datastore) GetActivity(ctx context.Context, activityID, baseURL string) (vocab.Type, error) {
	activities := d.client.Database("FediUni").Collection("activities")
	objectID, err := primitive.ObjectIDFromHex(activityID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ObjectID from")
	}
	filter := bson.D{{"_id", objectID}, {"id", fmt.Sprintf("%s/activity/%s", baseURL, activityID)}}
	var activity vocab.Type
	if err := activities.FindOne(ctx, filter).Decode(&activity); err != nil {
		return nil, err
	}
	if activity == nil {
		return nil, fmt.Errorf("unable to load user with _id=%q", activityID)
	}
	return activity, nil
}

func (d *Datastore) AddFollowerToActor(ctx context.Context, actorID, followerID string) error {
	users := d.client.Database("FediUni").Collection("followers")
	opts := options.Update().SetUpsert(true)
	log.Infof("Adding Follower=%q to Actor=%q", followerID, actorID)
	res, err := users.UpdateOne(ctx, bson.D{{"_id", actorID}}, bson.D{{"$addToSet", bson.D{{"followers", followerID}}}}, opts)
	if err != nil {
		return fmt.Errorf("failed to add follower to actor: got err=%v", err)
	}
	log.Infof("Inserted Document: got=%v", res)
	return nil
}
