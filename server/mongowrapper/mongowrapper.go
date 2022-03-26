package mongowrapper

import (
	"context"
	"errors"
	"fmt"
	"github.com/FediUni/FediUni/server/activity"
	"net/url"
	"strings"

	"github.com/FediUni/FediUni/server/actor"
	"github.com/FediUni/FediUni/server/user"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	log "github.com/golang/glog"
)

// Datastore wraps the MongoDB client and handles MongoDB operations.
type Datastore struct {
	client   *mongo.Client
	database string
	server   *url.URL
}

type followersCollection struct {
	Followers []string `bson:"followers"`
}

type followingCollection struct {
	Following []string `bson:"following"`
}

// NewDatastore returns an initialized Datastore which handles MongoDB operations.
func NewDatastore(client *mongo.Client, database string, server *url.URL) (*Datastore, error) {
	return &Datastore{
		client:   client,
		database: database,
		server:   server,
	}, nil
}

// GetActorByUsername returns an instance of Person from Mongo using Username.
func (d *Datastore) GetActorByUsername(ctx context.Context, username string) (vocab.ActivityStreamsPerson, error) {
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

func (d *Datastore) UpdateActor(ctx context.Context, username string, displayName string, summary string, profilePicture vocab.ActivityStreamsImage) error {
	actors := d.client.Database("FediUni").Collection("actors")
	filter := bson.D{{"preferredUsername", strings.ToLower(username)}}
	var values bson.D
	if profilePicture != nil {
		profile, err := streams.Serialize(profilePicture)
		if err != nil {
			return err
		}
		values = bson.D{
			{"$set", bson.D{{"name", displayName}}},
			{"$set", bson.D{{"summary", summary}}},
			{"$set", bson.D{{"icon", profile}}},
		}
	} else {
		values = bson.D{
			{"$set", bson.D{{"name", displayName}}},
			{"$set", bson.D{{"summary", summary}}},
		}
	}
	res, err := actors.UpdateOne(ctx, filter, values)
	if err != nil {
		return nil
	}
	log.Infof("Modified %d records", res.ModifiedCount)
	return nil
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
	followers := streams.NewActivityStreamsOrderedCollection()
	orderedFollowers := streams.NewActivityStreamsOrderedItemsProperty()
	followers.SetActivityStreamsOrderedItems(orderedFollowers)
	res := actors.FindOne(ctx, filter)
	if err := res.Err(); err == mongo.ErrNoDocuments {
		return followers, nil
	} else if err != nil {
		return nil, err
	}
	if err := res.Decode(&f); err != nil {
		return nil, err
	}
	totalItemsCount := streams.NewActivityStreamsTotalItemsProperty()
	totalItemsCount.Set(len(f.Followers))
	followers.SetActivityStreamsTotalItems(totalItemsCount)
	for _, follower := range f.Followers {
		followerID, err := url.Parse(follower)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %q as URL: got err=%v", follower, err)
		}
		orderedFollowers.AppendIRI(followerID)
	}
	return followers, nil
}

// GetFollowingByUsername returns an OrderedCollection of Following IDs.
func (d *Datastore) GetFollowingByUsername(ctx context.Context, username string) (vocab.ActivityStreamsOrderedCollection, error) {
	actor, err := d.GetActorByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to load actor username=%q: got err=%v", username, err)
	}
	actors := d.client.Database("FediUni").Collection("following")
	filter := bson.D{{"_id", actor.GetJSONLDId().Get().String()}}
	var f followingCollection
	if err := actors.FindOne(ctx, filter).Decode(&f); err != nil {
		return nil, err
	}
	following := streams.NewActivityStreamsOrderedCollection()
	orderedFollowing := streams.NewActivityStreamsOrderedItemsProperty()
	totalItemsCount := streams.NewActivityStreamsTotalItemsProperty()
	totalItemsCount.Set(len(f.Following))
	following.SetActivityStreamsTotalItems(totalItemsCount)
	for _, following := range f.Following {
		followerID, err := url.Parse(following)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %q as URL: got err=%v", following, err)
		}
		p := streams.NewActivityStreamsPerson()
		idProperty := streams.NewJSONLDIdProperty()
		idProperty.Set(followerID)
		p.SetJSONLDId(idProperty)
		orderedFollowing.AppendActivityStreamsPerson(p)
	}
	following.SetActivityStreamsOrderedItems(orderedFollowing)
	return following, nil
}

// GetLikedAsOrderedCollection returns an OrderedCollection of Like Activities.
func (d *Datastore) GetLikedAsOrderedCollection(ctx context.Context, username string) (vocab.ActivityStreamsOrderedCollection, error) {
	actor, err := d.GetActorByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to load actor username=%q: got err=%v", username, err)
	}
	liked := d.client.Database("FediUni").Collection("liked")
	filter := bson.D{{"actor", actor.GetJSONLDId().Get().String()}}
	likedCollection := streams.NewActivityStreamsOrderedCollection()
	likedURL, err := url.Parse(fmt.Sprintf("%s/actor/%s/liked", d.server.String(), username))
	if err != nil {
		return nil, fmt.Errorf("failed to parse liked URL: got err=%v", err)
	}
	id := streams.NewJSONLDIdProperty()
	id.Set(likedURL)
	likedCollection.SetJSONLDId(id)
	likedSize, err := liked.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to determine liked size: got err=%v", err)
	}
	totalItems := streams.NewActivityStreamsTotalItemsProperty()
	totalItems.Set(int(likedSize))
	likedCollection.SetActivityStreamsTotalItems(totalItems)
	if err != nil {
		return nil, fmt.Errorf("failed to determine first URL: got err=%v", err)
	}
	return likedCollection, nil
}

// GetLikesUsingObjectID returns an OrderedCollection of Like Activities.
func (d *Datastore) GetLikesUsingObjectID(ctx context.Context, activityID string) (vocab.ActivityStreamsOrderedCollection, error) {
	a, err := d.GetActivityByObjectID(ctx, activityID, d.server.String())
	if err != nil {
		return nil, fmt.Errorf("failed to load Activity: got err=%v", err)
	}
	idProperty := a.GetJSONLDId()
	if idProperty == nil {
		return nil, fmt.Errorf("failed to load an ID: got=%v", idProperty)
	}
	objectID := idProperty.Get()
	if objectID == nil {
		return nil, fmt.Errorf("failed to load an ID: got=%v", objectID)
	}
	return d.GetLikesAsOrderedCollection(ctx, objectID)
}

// GetLikesAsOrderedCollection returns all known likes of an ObjectID.
func (d *Datastore) GetLikesAsOrderedCollection(ctx context.Context, activityID *url.URL) (vocab.ActivityStreamsOrderedCollection, error) {
	if activityID == nil {
		return nil, fmt.Errorf("failed to receive an Activity ID: got=%v", activityID.String())
	}
	liked := d.client.Database("FediUni").Collection("liked")
	filter := bson.D{{"object", activityID.String()}}
	likedCollection := streams.NewActivityStreamsOrderedCollection()
	likedURL, err := url.Parse(fmt.Sprintf("%s/likes", activityID.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to parse liked URL: got err=%v", err)
	}
	id := streams.NewJSONLDIdProperty()
	id.Set(likedURL)
	likedCollection.SetJSONLDId(id)
	likedSize, err := liked.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to determine liked size: got err=%v", err)
	}
	totalItems := streams.NewActivityStreamsTotalItemsProperty()
	totalItems.Set(int(likedSize))
	likedCollection.SetActivityStreamsTotalItems(totalItems)
	if err != nil {
		return nil, fmt.Errorf("failed to determine first URL: got err=%v", err)
	}
	return likedCollection, nil
}

func (d *Datastore) LikeObject(ctx context.Context, objectID *url.URL, actorID *url.URL, activityID *url.URL) error {
	if objectID == nil {
		return fmt.Errorf("failed to receive an Object ID: got=%v", objectID)
	}
	if actorID == nil {
		return fmt.Errorf("failed to receive an Actor ID: got=%v", actorID)
	}
	if activityID == nil {
		return fmt.Errorf("failed to receive an Activity ID: got=%v", activityID)
	}
	users := d.client.Database("FediUni").Collection("liked")
	res, err := users.InsertOne(ctx, bson.M{
		"object":   objectID.String(),
		"actor":    actorID.String(),
		"activity": activityID.String(),
	})
	if err != nil {
		return fmt.Errorf("failed to insert activity: got err=%v", err)
	}
	log.Infof("Inserted Like Activity with ID=%q", res.InsertedID)
	return nil
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

func (d *Datastore) AddActivityToPublicInbox(ctx context.Context, activity vocab.Type, objectID primitive.ObjectID, isReply bool) error {
	activities := d.client.Database("FediUni").Collection("publicInbox")
	if activity.GetJSONLDId() == nil {
		id, err := url.Parse(fmt.Sprintf("%s/activity/%s", d.server.String(), objectID.Hex()))
		if err != nil {
			return err
		}
		idProperty := streams.NewJSONLDIdProperty()
		idProperty.Set(id)
		activity.SetJSONLDId(idProperty)
	}
	marshalledActivity, err := streams.Serialize(activity)
	if err != nil {
		return err
	}
	marshalledActivity["_id"] = objectID
	marshalledActivity["isReply"] = isReply
	// If the hostname matches the activity was created locally.
	if activity.GetJSONLDId().Get().Host == d.server.Host {
		marshalledActivity["isLocal"] = true
		marshalledActivity["isInstitute"] = true
	} else {
		marshalledActivity["isLocal"] = false
	}
	if _, ok := marshalledActivity["isInstitute"]; !ok {
		marshalledActivity["isInstitute"], err = d.IsHostSameInstitute(ctx, activity.GetJSONLDId().Get())
		if err != nil {
			return fmt.Errorf("failed to check if host is of the same institute: got err=%v", err)
		}
	}
	res, err := activities.InsertOne(ctx, marshalledActivity)
	if err != nil {
		return err
	}
	log.Infof("Inserted activity with _id=%q", res.InsertedID)
	return nil
}

func (d *Datastore) AddActivityToActivities(ctx context.Context, activity vocab.Type, objectID primitive.ObjectID) error {
	activities := d.client.Database("FediUni").Collection("activities")
	if activity.GetJSONLDId() == nil {
		id, err := url.Parse(fmt.Sprintf("%s/activity/%s", d.server.String(), objectID.Hex()))
		if err != nil {
			return err
		}
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
	log.Infof("Inserted activity with _id=%q", res.InsertedID)
	return nil
}

// GetPublicInboxAsOrderedCollection returns an orderedCollection.
// This collection is used to traverse the publicInbox collection in Mongo.
func (d *Datastore) GetPublicInboxAsOrderedCollection(ctx context.Context, local bool, institute bool) (vocab.ActivityStreamsOrderedCollection, error) {
	inbox := d.client.Database("FediUni").Collection("publicInbox")
	filter := bson.M{}
	filter["isReply"] = false
	if local {
		filter["isLocal"] = true
	}
	if institute {
		filter["isInstitute"] = true
	}
	inboxCollection := streams.NewActivityStreamsOrderedCollection()
	inboxURL, err := url.Parse(fmt.Sprintf("%s/inbox", d.server.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to parse outbox URL: got err=%v", err)
	}
	id := streams.NewJSONLDIdProperty()
	id.Set(inboxURL)
	inboxCollection.SetJSONLDId(id)
	inboxSize, err := inbox.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to determine inbox size: got err=%v", err)
	}
	totalItems := streams.NewActivityStreamsTotalItemsProperty()
	totalItems.Set(int(inboxSize))
	inboxCollection.SetActivityStreamsTotalItems(totalItems)
	first := streams.NewActivityStreamsFirstProperty()
	firstURL, err := url.Parse(fmt.Sprintf("%s?page=true&local=%t&institute=%t", inboxURL.String(), local, institute))
	if err != nil {
		return nil, fmt.Errorf("failed to determine first URL: got err=%v", err)
	}
	first.SetIRI(firstURL)
	inboxCollection.SetActivityStreamsFirst(first)
	last := streams.NewActivityStreamsLastProperty()
	lastURL, err := url.Parse(fmt.Sprintf("%s?page=true&min_id=0&local=%t&institute=%t", inboxURL.String(), local, institute))
	if err != nil {
		return nil, fmt.Errorf("failed to determine last URL: got err=%v", err)
	}
	last.SetIRI(lastURL)
	inboxCollection.SetActivityStreamsLast(last)
	return inboxCollection, nil
}

// GetPublicInbox paginates the inbox 20 activities at a time using IDs.
// ObjectIDs exceeding that maxID are ignored, and ObjectIDs under the min ID
// are ignored.
func (d *Datastore) GetPublicInbox(ctx context.Context, minID string, maxID string, local bool, institute bool) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	inbox := d.client.Database("FediUni").Collection("publicInbox")
	inboxURL, err := url.Parse(fmt.Sprintf("%s/inbox", d.server.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to parse outbox URL: got err=%v", err)
	}
	filter := bson.M{
		"isReply": false,
	}
	opts := options.Find().SetSort(bson.D{{"_id", -1}}).SetLimit(20)
	idFilters := bson.M{}
	if minID != "0" {
		id, err := primitive.ObjectIDFromHex(minID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse min_id: got err=%v", err)
		}
		idFilters["$gt"] = id
	}
	if maxID != "0" {
		id, err := primitive.ObjectIDFromHex(maxID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse max_id: got err=%v", err)
		}
		idFilters["$lt"] = id
	}
	if local {
		filter["isLocal"] = true
	}
	if institute {
		filter["isInstitute"] = true
	}
	if len(idFilters) != 0 {
		filter["_id"] = idFilters
	}
	cursor, err := inbox.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	page := streams.NewActivityStreamsOrderedCollectionPage()
	orderedItems := streams.NewActivityStreamsOrderedItemsProperty()
	activityResolver, err := streams.NewJSONResolver(func(ctx context.Context, c vocab.ActivityStreamsCreate) error {
		orderedItems.AppendActivityStreamsCreate(c)
		return nil
	}, func(ctx context.Context, a vocab.ActivityStreamsAnnounce) error {
		orderedItems.AppendActivityStreamsAnnounce(a)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create new type resolver: got err=%v", err)
	}
	var firstID primitive.ObjectID
	var lastID primitive.ObjectID
	totalItems := 0
	for cursor.Next(ctx) {
		totalItems++
		var m map[string]interface{}
		if err := cursor.Decode(&m); err != nil {
			return nil, err
		}
		if firstID.IsZero() {
			firstID = m["_id"].(primitive.ObjectID)
		}
		if err := activityResolver.Resolve(ctx, m); err != nil {
			return nil, err
		}
		lastID = m["_id"].(primitive.ObjectID)
	}
	totalItemsProperty := streams.NewActivityStreamsTotalItemsProperty()
	totalItemsProperty.Set(totalItems)
	page.SetActivityStreamsTotalItems(totalItemsProperty)
	previous := streams.NewActivityStreamsPrevProperty()
	previousURL, err := url.Parse(fmt.Sprintf("%s?page=true&local=%t&institute=%t&min_id=%s&max_id=%d", inboxURL.String(), local, institute, firstID.Hex(), 0))
	if err != nil {
		return nil, fmt.Errorf("failed to parse previous URL: got err=%v", err)
	}
	previous.SetIRI(previousURL)
	page.SetActivityStreamsPrev(previous)
	next := streams.NewActivityStreamsNextProperty()
	nextURL, err := url.Parse(fmt.Sprintf("%s?page=true&local=%t&institute=%t&min_id=%d&max_id=%s", inboxURL.String(), local, institute, 0, lastID.Hex()))
	if err != nil {
		return nil, fmt.Errorf("failed to parse next URL: got err=%v", err)

	}
	next.SetIRI(nextURL)
	page.SetActivityStreamsNext(next)
	inboxIRI := streams.NewJSONLDIdProperty()
	inboxID, err := url.Parse(fmt.Sprintf("%s?page=true&local=%t&institute=%t", inboxURL.String(), local, institute))
	if err != nil {
		return nil, fmt.Errorf("failed to parse inbox ID: got err=%v", err)
	}
	inboxIRI.Set(inboxID)
	page.SetActivityStreamsOrderedItems(orderedItems)
	return page, nil
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

func (d *Datastore) AddActivityToOutbox(ctx context.Context, activity vocab.Type, username string) error {
	outbox := d.client.Database("FediUni").Collection("outbox")
	m, err := streams.Serialize(activity)
	if err != nil {
		return err
	}
	m["sender"] = username
	res, err := outbox.InsertOne(ctx, m)
	if err != nil {
		return err
	}
	log.Infof("Inserted activity with _id=%q", res.InsertedID)
	return nil
}

// AddFollowerToActor adds the Follower ID to the Actor ID.
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

// AddActorToFollows adds the Actor ID to the Follower ID specified.
func (d *Datastore) AddActorToFollows(ctx context.Context, actorID, followerID string) error {
	log.Infof("Adding Follows ActorID=%q to Follower=%q", actorID, followerID)
	following := d.client.Database("FediUni").Collection("following")
	opts := options.Update().SetUpsert(true)
	res, err := following.UpdateOne(ctx, bson.D{{"_id", followerID}}, bson.D{{"$addToSet", bson.D{{"following", actorID}}}}, opts)
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
		m["recipient"] = strings.ToLower(userID)
		res, err := inbox.InsertOne(ctx, m)
		if err != nil {
			return err
		}
		log.Infof("Inserted Activity: got=%v", res)
	}
	return nil
}

func (d *Datastore) AddActivityToActorInbox(ctx context.Context, activity vocab.Type, username string, inReplyTo *url.URL) error {
	inbox := d.client.Database("FediUni").Collection("inbox")
	m, err := streams.Serialize(activity)
	if err != nil {
		return err
	}
	m["recipient"] = strings.ToLower(username)
	if inReplyTo != nil {
		m["isReply"] = true
	} else {
		m["isReply"] = false
	}
	// If the hostname matches the activity was created locally.
	m["isLocal"] = activity.GetJSONLDId().Get().Host == d.server.Host
	res, err := inbox.InsertOne(ctx, m)
	if err != nil {
		return err
	}
	log.Infof("Inserted Activity=%q: got=%v", activity.GetJSONLDId().Get().String(), res)
	return nil
}

func (d *Datastore) AddReplyToActivity(ctx context.Context, object vocab.Type, inReplyTo *url.URL) error {
	a, err := d.GetActivityByActivityID(ctx, inReplyTo.String())
	if err != nil {
		return err
	}
	var o vocab.ActivityStreamsObjectProperty
	switch a.GetTypeName() {
	case "Create":
		create, err := activity.ParseCreateActivity(ctx, a)
		if err != nil {
			return err
		}
		o = create.GetActivityStreamsObject()
	case "Announce":
		announce, err := activity.ParseAnnounceActivity(ctx, a)
		if err != nil {
			return err
		}
		o = announce.GetActivityStreamsObject()
	}
	if o == nil {
		return fmt.Errorf("cannot add reply to nil object")
	}
	for iter := o.Begin(); iter != o.End(); iter = iter.Next() {
		switch {
		case iter.IsActivityStreamsNote():
			note := iter.GetActivityStreamsNote()
			if note == nil {
				return fmt.Errorf("cannot add reply to nil note")
			}
			replies := note.GetActivityStreamsReplies()
			if replies == nil {
				return fmt.Errorf("reply property is nil")
			}
			collection := replies.GetActivityStreamsCollection()
			if collection == nil {
				return fmt.Errorf("collection property is nil")
			}
			items := collection.GetActivityStreamsItems()
			if items == nil {
				return fmt.Errorf("collection property is nil")
			}
			if err := items.AppendType(object); err != nil {
				return err
			}
		default:
			return fmt.Errorf("replying to Object of type=%q is unsupported", iter.GetType())
		}
	}
	return d.UpdateActivity(ctx, a, inReplyTo)
}

func (d *Datastore) UpdateActivity(ctx context.Context, activity vocab.Type, inReplyTo *url.URL) error {
	inbox := d.client.Database("FediUni").Collection("inbox")
	marshalledActivity, err := streams.Serialize(activity)
	if err != nil {
		return err
	}
	marshalledActivity["isReply"] = inReplyTo != nil
	// If the hostname matches the activity was created locally.
	marshalledActivity["isLocal"] = activity.GetJSONLDId().Get().Host == d.server.Host
	if _, err := inbox.ReplaceOne(ctx, bson.D{{"id", activity.GetJSONLDId().Get()}}, marshalledActivity); err != nil {
		return err
	}
	return nil
}

func (d *Datastore) GetActorOutboxAsOrderedCollection(ctx context.Context, username string) (vocab.ActivityStreamsOrderedCollection, error) {
	outbox := d.client.Database("FediUni").Collection("outbox")
	filter := bson.M{
		"$and": bson.A{
			bson.D{{"sender", username}},
			bson.D{{"type", bson.D{{"$in", bson.A{"Create", "Announce"}}}}},
		},
	}
	outboxCollection := streams.NewActivityStreamsOrderedCollection()
	outboxURL, err := url.Parse(fmt.Sprintf("%s/actor/%s/outbox", d.server.String(), username))
	if err != nil {
		return nil, fmt.Errorf("failed to parse outbox URL: got err=%v", err)
	}
	id := streams.NewJSONLDIdProperty()
	id.Set(outboxURL)
	outboxCollection.SetJSONLDId(id)
	outboxSize, err := outbox.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to determine inbox size: got err=%v", err)
	}
	totalItems := streams.NewActivityStreamsTotalItemsProperty()
	totalItems.Set(int(outboxSize))
	outboxCollection.SetActivityStreamsTotalItems(totalItems)
	first := streams.NewActivityStreamsFirstProperty()
	firstURL, err := url.Parse(fmt.Sprintf("%s?page=true", outboxURL.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to determine first URL: got err=%v", err)
	}
	first.SetIRI(firstURL)
	outboxCollection.SetActivityStreamsFirst(first)
	last := streams.NewActivityStreamsLastProperty()
	lastURL, err := url.Parse(fmt.Sprintf("%s?page=true&min_id=0", outboxURL.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to determine last URL: got err=%v", err)
	}
	last.SetIRI(lastURL)
	outboxCollection.SetActivityStreamsLast(last)
	return outboxCollection, nil
}

// GetActorOutbox paginates the inbox 20 activities at a time using IDs.
// ObjectIDs exceeding that maxID are ignored, and ObjectIDs under the min ID
// are ignored.
func (d *Datastore) GetActorOutbox(ctx context.Context, username, minID, maxID string) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	log.Infof("Searching for Recipient with Username=%q", username)
	outbox := d.client.Database("FediUni").Collection("outbox")
	filter := bson.M{
		"$and": bson.A{
			bson.D{{"sender", username}},
			bson.D{{"type", bson.D{{"$in", bson.A{"Create", "Announce"}}}}},
		},
	}
	outboxURL, err := url.Parse(fmt.Sprintf("%s/actor/%s/outbox", d.server.String(), username))
	if err != nil {
		return nil, fmt.Errorf("failed to parse outbox URL: got err=%v", err)
	}
	opts := options.Find().SetSort(bson.D{{"_id", -1}}).SetLimit(20)
	idFilters := bson.M{}
	if minID != "0" {
		id, err := primitive.ObjectIDFromHex(minID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse min_id: got err=%v", err)
		}
		idFilters["$gt"] = id
	}
	if maxID != "0" {
		id, err := primitive.ObjectIDFromHex(maxID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse max_id: got err=%v", err)
		}
		idFilters["$lt"] = id
	}
	if len(idFilters) != 0 {
		filter["_id"] = idFilters
	}
	cursor, err := outbox.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	page := streams.NewActivityStreamsOrderedCollectionPage()
	orderedItems := streams.NewActivityStreamsOrderedItemsProperty()
	activityResolver, err := streams.NewJSONResolver(func(ctx context.Context, c vocab.ActivityStreamsCreate) error {
		orderedItems.AppendActivityStreamsCreate(c)
		return nil
	}, func(ctx context.Context, a vocab.ActivityStreamsAnnounce) error {
		orderedItems.AppendActivityStreamsAnnounce(a)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create new type resolver: got err=%v", err)
	}
	var firstID primitive.ObjectID
	var lastID primitive.ObjectID
	totalItems := 0
	for cursor.Next(ctx) {
		totalItems++
		var m map[string]interface{}
		if err := cursor.Decode(&m); err != nil {
			return nil, err
		}
		if firstID.IsZero() {
			firstID = m["_id"].(primitive.ObjectID)
		}
		if err := activityResolver.Resolve(ctx, m); err != nil {
			return nil, err
		}
		lastID = m["_id"].(primitive.ObjectID)
	}
	totalItemsProperty := streams.NewActivityStreamsTotalItemsProperty()
	totalItemsProperty.Set(totalItems)
	page.SetActivityStreamsTotalItems(totalItemsProperty)
	previous := streams.NewActivityStreamsPrevProperty()
	previousURL, err := url.Parse(fmt.Sprintf("%s?page=true&min_id=%s&max_id=%d", outboxURL.String(), firstID.Hex(), 0))
	if err != nil {
		return nil, fmt.Errorf("failed to parse previous URL: got err=%v", err)
	}
	previous.SetIRI(previousURL)
	page.SetActivityStreamsPrev(previous)
	next := streams.NewActivityStreamsNextProperty()
	nextURL, err := url.Parse(fmt.Sprintf("%s?page=true&min_id=%d&max_id=%s", outboxURL.String(), 0, lastID.Hex()))
	if err != nil {
		return nil, fmt.Errorf("failed to parse next URL: got err=%v", err)

	}
	next.SetIRI(nextURL)
	page.SetActivityStreamsNext(next)
	outboxIRI := streams.NewJSONLDIdProperty()
	outboxID, err := url.Parse(fmt.Sprintf("%s?page=true", outboxURL.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to parse inbox ID: got err=%v", err)
	}
	outboxIRI.Set(outboxID)
	page.SetActivityStreamsOrderedItems(orderedItems)
	return page, nil
}

func (d *Datastore) GetActorInboxAsOrderedCollection(ctx context.Context, username string, local bool) (vocab.ActivityStreamsOrderedCollection, error) {
	inbox := d.client.Database("FediUni").Collection("inbox")
	var filter bson.D
	if local {
		filter = bson.D{
			{"recipient", strings.ToLower(username)},
			{"isReply", false},
			{"isLocal", local},
		}
	} else {
		filter = bson.D{
			{"recipient", strings.ToLower(username)},
			{"isReply", false},
		}
	}
	inboxCollection := streams.NewActivityStreamsOrderedCollection()
	inboxURL, err := url.Parse(fmt.Sprintf("%s/actor/%s/inbox", d.server.String(), username))
	if err != nil {
		return nil, fmt.Errorf("failed to parse outbox URL: got err=%v", err)
	}
	id := streams.NewJSONLDIdProperty()
	id.Set(inboxURL)
	inboxCollection.SetJSONLDId(id)
	inboxSize, err := inbox.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to determine inbox size: got err=%v", err)
	}
	totalItems := streams.NewActivityStreamsTotalItemsProperty()
	totalItems.Set(int(inboxSize))
	inboxCollection.SetActivityStreamsTotalItems(totalItems)
	first := streams.NewActivityStreamsFirstProperty()
	firstURL, err := url.Parse(fmt.Sprintf("%s?page=true&local=%t", inboxURL.String(), local))
	if err != nil {
		return nil, fmt.Errorf("failed to determine first URL: got err=%v", err)
	}
	first.SetIRI(firstURL)
	inboxCollection.SetActivityStreamsFirst(first)
	last := streams.NewActivityStreamsLastProperty()
	lastURL, err := url.Parse(fmt.Sprintf("%s?page=true&min_id=0&local=%t", inboxURL.String(), local))
	if err != nil {
		return nil, fmt.Errorf("failed to determine last URL: got err=%v", err)
	}
	last.SetIRI(lastURL)
	inboxCollection.SetActivityStreamsLast(last)
	return inboxCollection, nil
}

// GetActorInbox paginates the inbox 20 activities at a time using IDs.
// ObjectIDs exceeding that maxID are ignored, and ObjectIDs under the min ID
// are ignored.
func (d *Datastore) GetActorInbox(ctx context.Context, username, minID, maxID string, local bool) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	log.Infof("Searching for Recipient with Username=%q", username)
	inbox := d.client.Database("FediUni").Collection("inbox")
	inboxURL, err := url.Parse(fmt.Sprintf("%s/actor/%s/inbox", d.server.String(), username))
	if err != nil {
		return nil, fmt.Errorf("failed to parse outbox URL: got err=%v", err)
	}
	filter := bson.M{
		"recipient": strings.ToLower(username),
		"isReply":   false,
	}
	if local {
		filter["isLocal"] = true
	}
	opts := options.Find().SetSort(bson.D{{"_id", -1}}).SetLimit(20)
	idFilters := bson.M{}
	if minID != "0" {
		id, err := primitive.ObjectIDFromHex(minID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse min_id: got err=%v", err)
		}
		idFilters["$gt"] = id
	}
	if maxID != "0" {
		id, err := primitive.ObjectIDFromHex(maxID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse max_id: got err=%v", err)
		}
		idFilters["$lt"] = id
	}
	if len(idFilters) != 0 {
		filter["_id"] = idFilters
	}
	cursor, err := inbox.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	page := streams.NewActivityStreamsOrderedCollectionPage()
	orderedItems := streams.NewActivityStreamsOrderedItemsProperty()
	activityResolver, err := streams.NewJSONResolver(func(ctx context.Context, c vocab.ActivityStreamsCreate) error {
		orderedItems.AppendActivityStreamsCreate(c)
		return nil
	}, func(ctx context.Context, a vocab.ActivityStreamsAnnounce) error {
		orderedItems.AppendActivityStreamsAnnounce(a)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create new type resolver: got err=%v", err)
	}
	var firstID primitive.ObjectID
	var lastID primitive.ObjectID
	totalItems := 0
	for cursor.Next(ctx) {
		totalItems++
		var m map[string]interface{}
		if err := cursor.Decode(&m); err != nil {
			return nil, err
		}
		if firstID.IsZero() {
			firstID = m["_id"].(primitive.ObjectID)
		}
		if err := activityResolver.Resolve(ctx, m); err != nil {
			return nil, err
		}
		lastID = m["_id"].(primitive.ObjectID)
	}
	totalItemsProperty := streams.NewActivityStreamsTotalItemsProperty()
	totalItemsProperty.Set(totalItems)
	page.SetActivityStreamsTotalItems(totalItemsProperty)
	previous := streams.NewActivityStreamsPrevProperty()
	previousURL, err := url.Parse(fmt.Sprintf("%s?page=true&local=%t&min_id=%s&max_id=%d", inboxURL.String(), local, firstID.Hex(), 0))
	if err != nil {
		return nil, fmt.Errorf("failed to parse previous URL: got err=%v", err)
	}
	previous.SetIRI(previousURL)
	page.SetActivityStreamsPrev(previous)
	next := streams.NewActivityStreamsNextProperty()
	nextURL, err := url.Parse(fmt.Sprintf("%s?page=true&local=%t&min_id=%d&max_id=%s", inboxURL.String(), local, 0, lastID.Hex()))
	if err != nil {
		return nil, fmt.Errorf("failed to parse next URL: got err=%v", err)

	}
	next.SetIRI(nextURL)
	page.SetActivityStreamsNext(next)
	inboxIRI := streams.NewJSONLDIdProperty()
	inboxID, err := url.Parse(fmt.Sprintf("%s?page=true&local=%t", inboxURL.String(), local))
	if err != nil {
		return nil, fmt.Errorf("failed to parse inbox ID: got err=%v", err)
	}
	inboxIRI.Set(inboxID)
	page.SetActivityStreamsOrderedItems(orderedItems)
	return page, nil
}

func (d *Datastore) GetFollowerStatus(ctx context.Context, followerID, followedID string) (int, error) {
	following := d.client.Database("FediUni").Collection("following")
	filter := bson.M{"_id": followerID, "following": followedID}
	log.Infof("Checking If ActorID=%q follows ActorID=%q", followerID, followedID)
	res := following.FindOne(ctx, filter)
	log.Infof("Received Result=%v", res)
	err := res.Err()
	if err != nil && err != mongo.ErrNoDocuments {
		return 0, fmt.Errorf("Failed to retrieve follow status from Mongo: got err=%v", err)
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil
	}
	return 2, nil
}

func (d *Datastore) DeleteObjectFromAllInboxes(ctx context.Context, objectID *url.URL) error {
	if err := d.DeleteObjectFromPublicInbox(ctx, objectID); err != nil {
		return fmt.Errorf("failed to delete object from public inbox: got err=%v", err)
	}
	if err := d.DeleteObjectFromInbox(ctx, objectID); err != nil {
		return fmt.Errorf("failed to delete object from inbox: got err=%v", err)
	}
	return nil
}

func (d *Datastore) DeleteObjectFromPublicInbox(ctx context.Context, objectID *url.URL) error {
	public := d.client.Database("FediUni").Collection("publicInbox")
	res, err := public.DeleteMany(ctx, bson.D{
		{"$or", bson.A{
			bson.M{"id": objectID.String()},
			bson.M{"object.id": objectID}},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete object from public inbox: got err=%v", err)
	}
	log.Infof("Deleted %d documents from Public Inbox", res.DeletedCount)
	return nil
}

func (d *Datastore) DeleteObjectFromInbox(ctx context.Context, objectID *url.URL) error {
	inbox := d.client.Database("FediUni").Collection("inbox")
	res, err := inbox.DeleteMany(ctx, bson.D{
		{"$or", bson.A{
			bson.M{"id": objectID.String()},
			bson.M{"object.id": objectID}},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete object from public inbox: got err=%v", err)
	}
	log.Infof("Deleted %d documents from Public Inbox", res.DeletedCount)
	return nil
}

func (d *Datastore) AddHostToSameInstitute(ctx context.Context, instance *url.URL) error {
	sameInstitute := d.client.Database("FediUni").Collection("sameInstitute")
	opts := options.Update().SetUpsert(true)
	res, err := sameInstitute.UpdateOne(ctx, bson.M{"host": instance.Host}, bson.M{
		"host": instance.Host,
	}, opts)
	if err != nil {
		return fmt.Errorf("failed to insert host to University: got err=%v", err)
	}
	log.Infof("Modified %d documents and Upserted %d documents to sameInstitute", res.ModifiedCount, res.UpsertedCount)
	return nil
}

func (d *Datastore) IsHostSameInstitute(ctx context.Context, instance *url.URL) (bool, error) {
	sameInstitute := d.client.Database("FediUni").Collection("sameInstitute")
	res := sameInstitute.FindOne(ctx, bson.M{"host": instance.Host})
	if err := res.Err(); err == mongo.ErrNoDocuments {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}
