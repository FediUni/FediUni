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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, iri.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/activity+json")
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, iri.String(), nil)
	if err != nil {
		return nil, err
	}
	req.URL.Query().Add("resource", fmt.Sprintf("acct:%s@%s", actorID, iri.Host))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}
