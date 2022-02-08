package mongowrapper

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/FediUni/FediUni/activitypub/actor"
	"github.com/FediUni/FediUni/activitypub/user"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	log "github.com/golang/glog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Datastore wraps the MongoDB client and handles MongoDB operations.
type Datastore struct {
	client *mongo.Client
}

type followersCollection struct {
	Followers []string `bson:"followers"`
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

// GetFollowersByUsername returns an OrderedCollection of Follower IDs.
func (d *Datastore) GetFollowersByUsername(ctx context.Context, username string) (vocab.ActivityStreamsOrderedCollection, error) {
	actor, err := d.GetActorByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to load actor username=%q: got err=%v", username, err)
	}
	actors := d.client.Database("FediUni").Collection("followers")
	filter := bson.D{{"_id", actor.GetJSONLDId().Get().String()}}
	var f followersCollection
	if err := actors.FindOne(ctx, filter).Decode(&f); err != nil {
		return nil, err
	}
	followers := streams.NewActivityStreamsOrderedCollection()
	orderedFollowers := streams.NewActivityStreamsOrderedItemsProperty()
	for _, follower := range f.Followers {
		followerID, err := url.Parse(follower)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %q as URL: got err=%v", follower, err)
		}
		p := streams.NewActivityStreamsPerson()
		idProperty := streams.NewJSONLDIdProperty()
		idProperty.Set(followerID)
		p.SetJSONLDId(idProperty)
		orderedFollowers.AppendActivityStreamsPerson(p)
	}
	followers.SetActivityStreamsOrderedItems(orderedFollowers)
	return followers, nil
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

func (d *Datastore) GetUserByUsername(ctx context.Context, username string) (*user.User, error) {
	users := d.client.Database("FediUni").Collection("users")
	var user *user.User
	if err := users.FindOne(ctx, bson.D{{"username", strings.ToLower(username)}}).Decode(&user); err != nil {
		return nil, err
	}
	return user, nil
}

func (d *Datastore) AddActivityToSharedInbox(ctx context.Context, activity vocab.Type, baseURL string) error {
	activities := d.client.Database("FediUni").Collection("activities")
	objectID := primitive.NewObjectID()
	id, err := url.Parse(fmt.Sprintf("%s/activity/%s", baseURL, objectID.Hex()))
	if err != nil {
		return err
	}
	if activity.GetJSONLDId() == nil {
		idProperty := streams.NewJSONLDIdProperty()
		idProperty.Set(id)
		activity.SetJSONLDId(idProperty)
	}
	marshalledActivity, err := streams.Serialize(activity)
	if err != nil {
		return err
	}
	marshalledActivity["_id"] = objectID
	res, err := activities.InsertOne(ctx, marshalledActivity)
	if err != nil {
		return err
	}
	log.Infof("Inserted activity %q with _id=%q", id.String(), res.InsertedID)
	return nil
}

func (d *Datastore) GetActivityByObjectID(ctx context.Context, activityID, baseURL string) (vocab.Type, error) {
	activities := d.client.Database("FediUni").Collection("activities")
	objectID, err := primitive.ObjectIDFromHex(activityID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ObjectID from Hex=%q: got err=%v", activityID, err)
	}
	filter := bson.D{{"_id", objectID}}
	var m map[string]interface{}
	if err := activities.FindOne(ctx, filter).Decode(&m); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("unable to load activity with _id=%q", activityID)
	}
	activity, err := streams.ToType(ctx, m)
	if err != nil {
		return nil, fmt.Errorf("failed to create JSON vocab.ActivityStreamsObject resolver: got err=%v", err)
	}
	return activity, nil
}

func (d *Datastore) GetActivityByActivityID(ctx context.Context, activityID string) (vocab.Type, error) {
	activities := d.client.Database("FediUni").Collection("activities")
	filter := bson.D{{"id", activityID}}
	var m map[string]interface{}
	if err := activities.FindOne(ctx, filter).Decode(&m); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("unable to load activity with _id=%q", activityID)
	}
	activity, err := streams.ToType(ctx, m)
	if err != nil {
		return nil, fmt.Errorf("failed to create JSON vocab.ActivityStreamsObject resolver: got err=%v", err)
	}
	return activity, nil
}

func (d *Datastore) AddFollowerToActor(ctx context.Context, actorID, followerID string) error {
	log.Infof("Adding Follower=%q to Actor=%q", followerID, actorID)
	followers := d.client.Database("FediUni").Collection("followers")
	opts := options.Update().SetUpsert(true)
	res, err := followers.UpdateOne(ctx, bson.D{{"_id", actorID}}, bson.D{{"$addToSet", bson.D{{"followers", followerID}}}}, opts)
	if err != nil {
		return fmt.Errorf("failed to add follower to actor: got err=%v", err)
	}
	log.Infof("Inserted Document: got=%v", res)
	return nil
}

func (d *Datastore) RemoveFollowerFromActor(ctx context.Context, actorID, followerID string) error {
	users := d.client.Database("FediUni").Collection("followers")
	log.Infof("Removing Follower=%q from Actor=%q", followerID, actorID)
	res, err := users.UpdateOne(ctx, bson.D{{"_id", actorID}}, bson.D{{"$pull", bson.D{{"followers", followerID}}}})
	if err != nil {
		return fmt.Errorf("failed to remove follower from actor: got err=%v", err)
	}
	log.Infof("Inserted Document: got=%v", res)
	return nil
}

func (d *Datastore) AddObjectsToActorInbox(ctx context.Context, objects []vocab.Type, userID string) error {
	inbox := d.client.Database("FediUni").Collection("inbox")
	for _, object := range objects {
		m, err := streams.Serialize(object)
		if err != nil {
			return err
		}
		m["recipient"] = userID
		res, err := inbox.InsertOne(ctx, m)
		if err != nil {
			return err
		}
		log.Infof("Inserted Activity: got=%v", res)
	}
	return nil
}

func (d *Datastore) AddActivityToActorInbox(ctx context.Context, activity vocab.Type, userID string) error {
	inbox := d.client.Database("FediUni").Collection("inbox")
	m, err := streams.Serialize(activity)
	if err != nil {
		return err
	}
	m["recipient"] = userID
	res, err := inbox.InsertOne(ctx, m)
	if err != nil {
		return err
	}
	log.Infof("Inserted Activity: got=%v", res)
	return nil
}

func (d *Datastore) GetActorInbox(ctx context.Context, userID string) (vocab.ActivityStreamsOrderedCollection, error) {
	inbox := d.client.Database("FediUni").Collection("inbox")
	filter := bson.D{{"recipient", userID}}
	opts := options.Find().SetSort(bson.D{{"published", 1}})
	cursor, err := inbox.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	orderedCollection := streams.NewActivityStreamsOrderedCollection()
	orderedItems := streams.NewActivityStreamsOrderedItemsProperty()
	activityResolver, err := streams.NewJSONResolver(func(ctx context.Context, c vocab.ActivityStreamsCreate) error {
		orderedItems.AppendActivityStreamsCreate(c)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create new type resolver: got err=%v", err)
	}
	for cursor.Next(ctx) {
		var m map[string]interface{}
		if err := cursor.Decode(&m); err != nil {
			return nil, err
		}
		if err := activityResolver.Resolve(ctx, m); err != nil {
			return nil, err
		}
	}
	orderedCollection.SetActivityStreamsOrderedItems(orderedItems)
	return orderedCollection, nil
}
