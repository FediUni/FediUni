package client

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/activity"
	"github.com/FediUni/FediUni/activitypub/validation"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	log "github.com/golang/glog"
	"io/ioutil"
	"net/http"
	"net/url"
)

type Client struct {
	InstanceURL *url.URL
}

func NewClient(instanceURL *url.URL) *Client {
	return &Client{
		InstanceURL: instanceURL,
	}
}

func (c *Client) FetchRemoteObject(ctx context.Context, iri *url.URL) (vocab.Type, error) {
	if iri == nil {
		return nil, fmt.Errorf("failed to receive IRI: got=%v", iri)
	}
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
	marshalledObject, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(marshalledObject, &m); err != nil {
		return nil, err
	}
	return streams.ToType(ctx, m)
}

func (c *Client) PostToInbox(ctx context.Context, inbox *url.URL, object vocab.Type, keyID string, privateKey *rsa.PrivateKey) error {
	marshalledFollowActivity, err := activity.JSON(object)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inbox.String(), bytes.NewBuffer(marshalledFollowActivity))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/ld+json")
	req, err = validation.SignRequestWithDigest(req, c.InstanceURL, keyID, privateKey, marshalledFollowActivity)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to post object to inbox=%q", inbox.String())
	}
	return nil
}

func (c *Client) WebfingerLookup(ctx context.Context, iri *url.URL, actorID string) ([]byte, error) {
	log.Infof("Performing Webfinger Lookup: %q", iri.String())
	res, err := http.DefaultClient.Get(fmt.Sprintf("%s?resource=%s", iri.String(), fmt.Sprintf("acct:%s@%s", actorID, iri.Host)))
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
