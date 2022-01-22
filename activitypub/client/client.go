package client

import (
	"context"
	"encoding/json"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	"io/ioutil"
	"net/http"
	"net/url"
)

type Client struct {
}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) FetchRemoteActivity(ctx context.Context, iri *url.URL) (vocab.Type, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, iri.String(), nil)
	if err != nil {
		return nil, err
	}
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
