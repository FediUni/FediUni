package client

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/go-fed/activity/streams"
	"github.com/google/go-cmp/cmp"
)

type TestCache struct {
	cache map[string][]byte
}

func (c *TestCache) Store(key string, value []byte, expiration time.Duration) error {
	return nil
}

func (c *TestCache) Load(key string) ([]byte, error) {
	return c.cache[key], nil
}

func TestFetchRemoteObject(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value map[string]interface{}
	}{
		{
			name: "Test loading remote object from cache",
			key:  "https://non-existent-site.com/actor/fake-actor",
			value: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       "https://non-existent-site.com/actor/fake-actor",
				"type":     "Object",
			},
		},
	}
	for _, test := range tests {
		marshalledValue, err := json.Marshal(test.value)
		if err != nil {
			t.Fatalf("failed to marshal value: got err=%v", err)
		}
		t.Run(test.name, func(t *testing.T) {
			client := &Client{
				Cache: &TestCache{
					cache: map[string][]byte{
						test.key: marshalledValue,
					},
				},
			}
			iri, err := url.Parse(test.key)
			if err != nil {
				t.Errorf("failed to parse key=%q: got err=%v", test.key, err)
			}
			object, err := client.FetchRemoteObject(context.Background(), iri, false, 0, 1)
			if err != nil {
				t.Errorf("client.FetchRemoteObject(): failed to fetch remote object: got err=%v", err)
			}
			gotObject, err := streams.Serialize(object)
			if d := cmp.Diff(gotObject, test.value); d != "" {
				t.Errorf("client.FetchRemoteObject(): returned a different object: (-want +got) %s", d)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name  string
		key   []string
		value []map[string]interface{}
		want  map[string]interface{}
	}{
		{
			name: "Test dereference Actor IRI in Create Activity",
			key: []string{
				"https://non-existent-site.com/activity/fake-create",
				"https://non-existent-site.com/actor/fake-actor",
				"https://non-existent-site.com/object/fake-note",
			},
			value: []map[string]interface{}{
				{
					"@context": "https://www.w3.org/ns/activitystreams",
					"id":       "https://non-existent-site.com/actor/fake-create",
					"type":     "Create",
					"actor":    "https://non-existent-site.com/actor/fake-actor",
					"object":   "https://non-existent-site.com/object/fake-note",
				},
				{
					"@context": "https://www.w3.org/ns/activitystreams",
					"id":       "https://non-existent-site.com/actor/fake-actor",
					"type":     "Person",
				},
				{
					"@context":     "https://www.w3.org/ns/activitystreams",
					"id":           "https://non-existent-site.com/object/fake-note",
					"type":         "Note",
					"attributedTo": "https://non-existent-site.com/actor/fake-actor",
				},
			},
			want: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       "https://non-existent-site.com/actor/fake-create",
				"type":     "Create",
				"actor": map[string]interface{}{
					"id":   "https://non-existent-site.com/actor/fake-actor",
					"type": "Person",
				},
				"object": map[string]interface{}{
					"id":   "https://non-existent-site.com/object/fake-note",
					"type": "Note",
					"attributedTo": map[string]interface{}{
						"id":   "https://non-existent-site.com/actor/fake-actor",
						"type": "Person",
					},
				},
			},
		},
	}
	for _, test := range tests {
		marshalledCreate, err := json.Marshal(test.value[0])
		if err != nil {
			t.Fatalf("failed to marshal value: got err=%v", err)
		}
		marshalledActor, err := json.Marshal(test.value[1])
		if err != nil {
			t.Fatalf("failed to marshal value: got err=%v", err)
		}
		marshalledNote, err := json.Marshal(test.value[2])
		if err != nil {
			t.Fatalf("failed to marshal value: got err=%v", err)
		}
		t.Run(test.name, func(t *testing.T) {
			client := &Client{
				Cache: &TestCache{
					cache: map[string][]byte{
						test.key[0]: marshalledCreate,
						test.key[1]: marshalledActor,
						test.key[2]: marshalledNote,
					},
				},
			}
			iri, err := url.Parse(test.key[0])
			if err != nil {
				t.Errorf("failed to parse key=%q: got err=%v", test.key, err)
			}
			object, err := client.FetchRemoteObject(context.Background(), iri, false, 0, 1)
			if err != nil {
				t.Errorf("client.FetchRemoteObject(): failed to fetch remote object: got err=%v", err)
			}
			gotObject, err := streams.Serialize(object)
			if d := cmp.Diff(gotObject, test.want); d != "" {
				t.Errorf("client.FetchRemoteObject(): returned a different object: (-want +got) %s", d)
			}
		})
	}
}
