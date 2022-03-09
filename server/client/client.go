package client

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/FediUni/FediUni/server/activity"
	"github.com/FediUni/FediUni/server/actor"
	"github.com/FediUni/FediUni/server/object"
	"github.com/FediUni/FediUni/server/validation"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	"github.com/go-redis/redis"
	log "github.com/golang/glog"
)

const (
	cacheTime = 10 * time.Minute
)

type WebfingerResponse struct {
	Subject string          `json:"subject"`
	Links   []WebfingerLink `json:"links"`
}

type WebfingerLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type"`
	Href string `json:"href"`
}

// Cache defines an interface for caching and reading the received objects.
type Cache interface {
	Store(string, []byte, time.Duration) error
	Load(string) ([]byte, error)
}

type Client struct {
	InstanceURL *url.URL
	Cache       Cache
}

// RedisCache wraps a Redis Client to implement the Cache interface.
type RedisCache struct {
	redis *redis.Client
}

// Store adds the provided value using the provided key to the Redis store.
func (c *RedisCache) Store(key string, value []byte, expiration time.Duration) error {
	return c.redis.Set(key, value, expiration).Err()
}

// Load reads the provided value using provided key from the Redis store.
func (c *RedisCache) Load(key string) ([]byte, error) {
	value, err := c.redis.Get(key).Result()
	if err != nil {
		return nil, err
	}
	return []byte(value), nil
}

// NewClient initializes a client to retrieve and cache ActivityPub objects.
func NewClient(instance *url.URL, address string, password string) *Client {
	return &Client{
		InstanceURL: instance,
		Cache: &RedisCache{
			redis.NewClient(&redis.Options{
				Addr:     address,
				Password: password,
			}),
		},
	}
}

// FetchRemoteObject retrieves the resource located at the provided IRI.
// The client makes use of caching but this can be overriden.
func (c *Client) FetchRemoteObject(ctx context.Context, iri *url.URL, forceUpdate bool, depth int8, dereferenceReplies bool) (vocab.Type, error) {
	prefix := fmt.Sprintf("(Depth = %d)", depth)
	if iri == nil {
		return nil, fmt.Errorf("failed to receive IRI: got=%v", iri)
	}
	var marshalledObject []byte
	var err error
	if !forceUpdate {
		log.Infof("%s Reading ObjectID=%q from cache...", prefix, iri.String())
		marshalledObject, err = c.Cache.Load(iri.String())
		if err != nil {
			log.Errorf("%s Redis failed to load ObjectID=%q from cache: got err=%v", prefix, iri.String(), err)
		}
	}
	if len(marshalledObject) == 0 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, iri.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/activity+json")
		log.Infof("%s Fetching resource at ID=%q", prefix, iri.String())
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		marshalledObject, err = ioutil.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}
		if err := c.Cache.Store(iri.String(), marshalledObject, cacheTime); err != nil {
			log.Errorf("%s Failed to cache ObjectID=%q: got err=%v", prefix, err, iri.String())
		}
	}
	var m map[string]interface{}
	if err := json.Unmarshal(marshalledObject, &m); err != nil {
		log.Infof("%s Storing ObjectID=%q in cache...", prefix, iri.String())
		return nil, err
	}
	object, err := streams.ToType(ctx, m)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve object to type: got err=%v", err)
	}
	switch typeName := object.GetTypeName(); typeName {
	case "Create":
		create, err := activity.ParseCreateActivity(ctx, object)
		if err != nil {
			return nil, err
		}
		if err := c.Create(ctx, create, depth, dereferenceReplies); err != nil {
			return nil, err
		}
	case "Announce":
		announce, err := activity.ParseAnnounceActivity(ctx, object)
		if err != nil {
			return nil, err
		}
		if err := c.Announce(ctx, announce, depth, dereferenceReplies); err != nil {
			return nil, err
		}
	}

	return object, nil
}

func (c *Client) ResolveActorIdentifierToID(ctx context.Context, identifier string) (*url.URL, error) {
	isIdentifier, err := actor.IsIdentifier(identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to receive an identifier: got=%q", identifier)
	}
	if !isIdentifier {
		return nil, fmt.Errorf("failed to receive an identifier: got=%q", identifier)
	}
	splitIdentifier := strings.Split(identifier, "@")
	username := splitIdentifier[1]
	domain := splitIdentifier[2]
	webfingerResponse, err := c.WebfingerLookup(ctx, domain, username)
	if err != nil {
		log.Errorf("failed to lookup actor=%q, got webfinger err=%v", fmt.Sprintf("@%s@%s", username, domain), err)
		return nil, fmt.Errorf("failed to fetch remote actor: got err=%v", err)
	}
	var actorID *url.URL
	for _, link := range webfingerResponse.Links {
		if !strings.Contains(link.Type, "application/activity+json") && !strings.Contains(link.Type, "application/ld+json") {
			continue
		}
		if actorID, err = url.Parse(link.Href); err != nil {
			log.Errorf("failed to load actorID: got err=%v", err)
			return nil, fmt.Errorf("failed to parse the URL from href=%q: got err=%v", link.Href, err)
		}
		break
	}
	return actorID, nil
}

// FetchRemotePerson performs a Webfinger lookup and returns a Person.
func (c *Client) FetchRemotePerson(ctx context.Context, identifier string) (vocab.ActivityStreamsPerson, error) {
	actorID, err := c.ResolveActorIdentifierToID(ctx, identifier)
	if err != nil {
		return nil, err
	}
	actor, err := c.FetchRemoteObject(ctx, actorID, false, 0, false)
	if err != nil {
		return nil, err
	}
	var person vocab.ActivityStreamsPerson
	resolver, err := streams.NewTypeResolver(func(ctx context.Context, p vocab.ActivityStreamsPerson) error {
		person = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := resolver.Resolve(ctx, actor); err != nil {
		return nil, err
	}
	return person, err
}

// FetchRemotePersonWithID uses the provided ID to retrieve a Person.
func (c *Client) FetchRemotePersonWithID(ctx context.Context, personID *url.URL) (vocab.ActivityStreamsPerson, error) {
	actor, err := c.FetchRemoteObject(ctx, personID, false, 0, false)
	if err != nil {
		return nil, err
	}
	var person vocab.ActivityStreamsPerson
	resolver, err := streams.NewTypeResolver(func(ctx context.Context, p vocab.ActivityStreamsPerson) error {
		person = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := resolver.Resolve(ctx, actor); err != nil {
		return nil, err
	}
	return person, err
}

// FetchRemoteActor performs a Webfinger lookup and returns an Actor.
func (c *Client) FetchRemoteActor(ctx context.Context, identifier string) (vocab.Type, error) {
	actorID, err := c.ResolveActorIdentifierToID(ctx, identifier)
	if err != nil {
		return nil, err
	}
	return c.FetchRemoteObject(ctx, actorID, false, 0, false)
}

func (c *Client) FetchFollowers(ctx context.Context, identifier string) ([]vocab.ActivityStreamsPerson, error) {
	person, err := c.FetchRemotePerson(ctx, identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote person=%q: got err=%v", identifier, err)
	}
	var followers vocab.Type
	switch f := person.GetActivityStreamsFollowers(); {
	case f.IsIRI():
		followers, err = c.FetchRemoteObject(ctx, f.GetIRI(), false, 0, false)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unexpected type of Followers field: got type=%q", f.GetType())
	}
	var orderedCollection vocab.ActivityStreamsOrderedCollection
	resolver, err := streams.NewTypeResolver(func(ctx context.Context, c vocab.ActivityStreamsOrderedCollection) error {
		orderedCollection = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := resolver.Resolve(ctx, followers); err != nil {
		return nil, err
	}
	var dereferencedFollowers []vocab.ActivityStreamsPerson
	resolver, err = streams.NewTypeResolver(func(ctx context.Context, p vocab.ActivityStreamsPerson) error {
		dereferencedFollowers = append(dereferencedFollowers, p)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create person resolver: got err=%v", err)
	}
	for iter := orderedCollection.GetActivityStreamsOrderedItems().Begin(); iter != nil; iter = iter.Next() {
		switch {
		case iter.IsIRI():
			o, err := c.FetchRemoteObject(ctx, iter.GetIRI(), false, 0, false)
			if err != nil {
				log.Errorf("failed to fetch remote object: got err=%v", err)
				continue
			}
			if err := resolver.Resolve(ctx, o); err != nil {
				log.Errorf("failed to resolve remote object: got err=%v", err)
				continue
			}
		case iter.IsActivityStreamsPerson():
			dereferencedFollowers = append(dereferencedFollowers, iter.GetActivityStreamsPerson())
		default:
			log.Infof("Ignoring follower of type=%q", iter.GetType())
		}
	}
	log.Infof("Loaded %d followers from %q", len(dereferencedFollowers), identifier)
	return dereferencedFollowers, nil
}

func (c *Client) PostToInbox(ctx context.Context, inbox *url.URL, object vocab.Type, keyID string, privateKey *rsa.PrivateKey) error {
	marshalledActivity, err := activity.JSON(object)
	if err != nil {
		return err
	}
	log.Infof("Creating Request to URL=%q", inbox.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inbox.String(), bytes.NewBuffer(marshalledActivity))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/ld+json")
	log.Infof("Signing Request...")
	req, err = validation.SignRequestWithDigest(req, c.InstanceURL, keyID, privateKey, marshalledActivity)
	if err != nil {
		return err
	}
	log.Infof("Sending Request...")
	dump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		log.Errorf("Can't dump: %v", err)
	} else {
		log.Infof("%s", dump)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read from response body: got err=%v", err)
	}
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusAccepted {
		return fmt.Errorf("failed to post object to inbox=%q: got StatusCode=%d, Body=%s", inbox.String(), res.StatusCode, string(body))
	}
	log.Infof("Successfully POSTed to Inbox=%q: got StatusCode=%d and Body=%q", inbox.String(), res.StatusCode, string(body))
	return nil
}

func (c *Client) WebfingerLookup(ctx context.Context, domain string, actorID string) (*WebfingerResponse, error) {
	webfingerURL, err := url.Parse(fmt.Sprintf("https://%s/.well-known/webfinger?resource=%s", domain, fmt.Sprintf("acct:%s@%s", actorID, domain)))
	if err != nil {
		return nil, err
	}
	log.Infof("Performing Webfinger Lookup: %q", webfingerURL.String())
	res, err := http.DefaultClient.Get(webfingerURL.String())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("received empty body: %q", string(body))
	}
	var webfingerResponse *WebfingerResponse
	if err := json.Unmarshal(body, &webfingerResponse); err != nil {
		log.Errorf("failed to unmarshal webfinger response: got err=%v", err)
		return nil, fmt.Errorf("failed to unmarshal the WebfingerResponse: got err=%v", err)
	}
	return webfingerResponse, nil
}

// Create dereferences the actor and object fields of the activity.
func (c *Client) Create(ctx context.Context, create vocab.ActivityStreamsCreate, depth int8, dereferenceReplies bool) error {
	if depth == 5 {
		log.Infof("Depth = %d. Returning...", depth)
	}
	// Dereference all Actors of type Person in actor field.
	for iter := create.GetActivityStreamsActor().Begin(); iter != nil; iter = iter.Next() {
		var actorID *url.URL
		switch {
		case iter.IsIRI():
			actorID = iter.GetIRI()
		case iter.IsActivityStreamsPerson():
			actorID = iter.GetActivityStreamsPerson().GetJSONLDId().Get()
		}
		person, err := c.FetchRemotePersonWithID(ctx, actorID)
		if err != nil {
			return fmt.Errorf("failed to resolve Actor ID=%q: got err=%v", actorID, err)
		}
		iter.SetActivityStreamsPerson(person)
	}
	for iter := create.GetActivityStreamsObject().Begin(); iter != nil; iter = iter.Next() {
		if !iter.IsIRI() {
			continue
		}
		objectID := iter.GetIRI()
		objectRetrieved, err := c.FetchRemoteObject(ctx, objectID, false, depth+1, dereferenceReplies)
		if err != nil {
			return fmt.Errorf("failed to resolve Object ID=%q: got err=%v", objectID.String(), err)
		}
		switch objectRetrieved.GetTypeName() {
		case "Note":
			note, err := object.ParseNote(ctx, objectRetrieved)
			if err != nil {
				return fmt.Errorf("failed to parse Object ID=%q as Note: got err=%v", objectID.String(), err)
			}
			if err := c.Note(ctx, note, depth+1, dereferenceReplies); err != nil {
				return fmt.Errorf("failed to dereference Note ID=%q: got err=%v", objectID.String(), err)
			}
			iter.SetActivityStreamsNote(note)
		}
	}
	return nil
}

// Announce dereferences the actor and object fields of the activity.
func (c *Client) Announce(ctx context.Context, announce vocab.ActivityStreamsAnnounce, depth int8, dereferenceReplies bool) error {
	if depth == 5 {
		log.Infof("Depth = %d. Returning...", depth)
	}
	for iter := announce.GetActivityStreamsActor().Begin(); iter != nil; iter = iter.Next() {
		var actorID *url.URL
		switch {
		case iter.IsIRI():
			actorID = iter.GetIRI()
		case iter.IsActivityStreamsPerson():
			actorID = iter.GetActivityStreamsPerson().GetJSONLDId().Get()
		}
		person, err := c.FetchRemotePersonWithID(ctx, actorID)
		if err != nil {
			return fmt.Errorf("failed to resolve Actor ID=%q: got err=%v", actorID, err)
		}
		iter.SetActivityStreamsPerson(person)
	}
	for iter := announce.GetActivityStreamsObject().Begin(); iter != nil; iter = iter.Next() {
		if !iter.IsIRI() {
			continue
		}
		objectID := iter.GetIRI()
		objectRetrieved, err := c.FetchRemoteObject(ctx, objectID, false, depth+1, dereferenceReplies)
		if err != nil {
			return fmt.Errorf("failed to resolve Object ID=%q: got err=%v", objectID.String(), err)
		}
		switch objectRetrieved.GetTypeName() {
		case "Note":
			note, err := object.ParseNote(ctx, objectRetrieved)
			if err != nil {
				return fmt.Errorf("failed to parse Object ID=%q as Note: got err=%v", objectID.String(), err)
			}
			if err := c.Note(ctx, note, depth+1, dereferenceReplies); err != nil {
				return fmt.Errorf("failed to dereference Note ID=%q: got err=%v", objectID.String(), err)
			}
			iter.SetActivityStreamsNote(note)
		}
	}
	return nil
}

// Note dereferences the actors of type Person in attributedTo.
func (c *Client) Note(ctx context.Context, note vocab.ActivityStreamsNote, depth int8, dereferenceReplies bool) error {
	prefix := fmt.Sprintf("(Depth = %d)", depth)
	if depth == 5 {
		log.Infof("Depth = %d. Returning...", depth)
	}
	if note == nil {
		return fmt.Errorf("error in dereferencing note: got note=%v", note)
	}
	if note.GetActivityStreamsAttributedTo() == nil {
		return fmt.Errorf("error in dereferencing note: got attributedTo=%v", note.GetActivityStreamsAttributedTo())
	}
	for iter := note.GetActivityStreamsAttributedTo().Begin(); iter != nil; iter = iter.Next() {
		switch {
		case iter.IsIRI():
			actorID := iter.GetIRI()
			actorRetrieved, err := c.FetchRemoteObject(ctx, actorID, false, depth+1, false)
			if err != nil {
				return fmt.Errorf("failed to resolve Actor ID=%q: got err=%v", actorID.String(), err)
			}
			switch actorRetrieved.GetTypeName() {
			case "Person":
				person, err := actor.ParsePerson(ctx, actorRetrieved)
				if err != nil {
					return fmt.Errorf("failed to parse Actor ID=%q as Person: got err=%v", actorID.String(), err)
				}
				iter.SetActivityStreamsPerson(person)
			}
		case iter.IsActivityStreamsPerson():
			p := iter.GetActivityStreamsPerson()
			person, err := c.FetchRemotePersonWithID(ctx, p.GetJSONLDId().Get())
			if err != nil {
				return fmt.Errorf("failed to resolve Actor ID=%q: got err=%v", p.GetJSONLDId().Get().String(), err)
			}
			iter.SetActivityStreamsPerson(person)
		}
	}
	if !dereferenceReplies {
		log.Errorf("%s Skipping dereferencing replies...", prefix)
		return nil
	}
	log.Infof("%s Attempting to dereference replies on Note ID=%q", prefix, note.GetJSONLDId().Get().String())
	repliesProperty := note.GetActivityStreamsReplies()
	if repliesProperty == nil {
		log.Errorf("%s Cannot dereference replies on Note ID=%q as replies=%v", prefix, note.GetJSONLDId().Get().String(), repliesProperty)
		return nil
	}
	collection := repliesProperty.GetActivityStreamsCollection()
	if collection == nil {
		log.Errorf("%s Cannot dereference replies on Note ID=%q as collection=%v", prefix, note.GetJSONLDId().Get().String(), collection)
		return nil
	}
	log.Infof("Handling first page of replies to Note ID=%q", note.GetJSONLDId().Get().String())
	first := collection.GetActivityStreamsFirst()
	if first == nil {
		log.Errorf("%s Cannot dereference replies on Note ID=%q as first page=%v", prefix, note.GetJSONLDId().Get().String(), first)
		return nil
	}
	if first.IsIRI() {
		page, err := c.FetchRemoteObject(ctx, first.GetIRI(), false, depth+1, false)
		if err != nil {
			return fmt.Errorf("failed to fetch first page of replies collection")
		}
		if err := first.SetType(page); err != nil {
			return fmt.Errorf("failed to dereference first page of replies collection: got err=%v", err)
		}
	}
	firstPage := first.GetActivityStreamsCollectionPage()
	// Retrieve top level replies for now.
	next := firstPage.GetActivityStreamsNext()
	log.Infof("%s Handling next page of replies to Note ID=%q", prefix, note.GetJSONLDId().Get().String())
	if next == nil {
		log.Infof("%s Cannot dereference replies on Note ID=%q as next=%v", prefix, note.GetJSONLDId().Get().String(), next)
		return nil
	}
	if next.IsIRI() {
		page, err := c.FetchRemoteObject(ctx, next.GetIRI(), false, depth+1, false)
		if err != nil {
			return fmt.Errorf("failed to fetch next page of replies collection")
		}
		if err := next.SetType(page); err != nil {
			return fmt.Errorf("failed to dereference next page of replies collection: got err=%v", err)
		}
	}
	nextPage := next.GetActivityStreamsCollectionPage()
	if nextPage == nil {
		return nil
	}
	items := nextPage.GetActivityStreamsItems()
	if items == nil {
		log.Infof("%s No items to dereference", prefix)
		return nil
	}
	for iter := items.Begin(); iter != items.End(); iter = iter.Next() {
		var reply vocab.Type
		switch {
		case iter.IsIRI():
			o, err := c.FetchRemoteObject(ctx, iter.GetIRI(), false, depth+1, false)
			if err != nil {
				return fmt.Errorf("failed to fetch reply ID=%q", reply.GetJSONLDId())
			}
			reply = o
		default:
			reply = iter.GetType()
		}
		if reply == nil {
			continue
		}
		switch reply.GetTypeName() {
		case "Note":
			n, err := object.ParseNote(ctx, reply)
			if err != nil {
				log.Errorf("failed to parse note: got err=%v", err)
				continue
			}
			if err := c.Note(ctx, n, depth+1, false); err != nil {
				log.Errorf("failed to dereference note: got err=%v", err)
				continue
			}
			if err := iter.SetType(n); err != nil {
				return fmt.Errorf("failed to dereference reply: got err=%v", err)
			}
		}
	}
	return nil
}
