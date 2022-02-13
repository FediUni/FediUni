package client

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/FediUni/FediUni/activitypub/activity"
	"github.com/FediUni/FediUni/activitypub/validation"
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
func (c *Client) FetchRemoteObject(ctx context.Context, iri *url.URL, forceUpdate bool) (vocab.Type, error) {
	if iri == nil {
		return nil, fmt.Errorf("failed to receive IRI: got=%v", iri)
	}
	var marshalledObject []byte
	var err error
	if !forceUpdate {
		log.Infof("Reading ObjectID=%q from cache...", iri.String())
		marshalledObject, err = c.Cache.Load(iri.String())
		if err != nil {
			log.Errorf("Redis failed to load ObjectID=%q from cache: got err=%v", iri.String(), err)
		}
	}
	if len(marshalledObject) == 0 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, iri.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/activity+json")
		log.Infof("Fetching resource at ID=%q", iri.String())
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
			log.Errorf("Failed to cache ObjectID=%q: got err=%v", err, iri.String())
		}
	}
	var m map[string]interface{}
	if err := json.Unmarshal(marshalledObject, &m); err != nil {
		log.Infof("Storing ObjectID=%q in cache...", iri.String())
		return nil, err
	}
	return streams.ToType(ctx, m)
}

// FetchRemoteActor performs a Webfinger lookup and returns an Actor.
func (c *Client) FetchRemoteActor(ctx context.Context, username, domain string) (vocab.Type, error) {
	res, err := c.WebfingerLookup(ctx, domain, username)
	if err != nil {
		log.Errorf("failed to lookup actor=%q, got err=%v", fmt.Sprintf("@%s@%s", username, domain), err)
		return nil, fmt.Errorf("failed to fetch remote actor: got err=%v", err)
	}
	var webfingerResponse *WebfingerResponse
	if err := json.Unmarshal(res, &webfingerResponse); err != nil {
		log.Errorf("failed to unmarshal webfinger response: got err=%v", err)
		return nil, fmt.Errorf("failed to unmarshal the WebfingerResponse: got err=%v", err)
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
	return c.FetchRemoteObject(ctx, actorID, false)
}

func (c *Client) PostToInbox(ctx context.Context, inbox *url.URL, object vocab.Type, keyID string, privateKey *rsa.PrivateKey) error {
	log.Infof("Marshalling Activity...")
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

func (c *Client) WebfingerLookup(ctx context.Context, domain string, actorID string) ([]byte, error) {
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
	return body, nil
}
