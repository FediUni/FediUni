package actor

import (
	"context"
	"fmt"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	"github.com/google/go-cmp/cmp"
	"net/url"
	"testing"
)

type testDatastore struct {
	Actors map[string]vocab.ActivityStreamsPerson
}

func (d testDatastore) GetActorByUsername(_ context.Context, username string) (vocab.ActivityStreamsPerson, error) {
	actor := d.Actors[username]
	if actor == nil {
		return nil, fmt.Errorf("failed to load Actor: got=%v", actor)
	}
	return actor, nil
}

func (d testDatastore) GetFollowersByUsername(ctx context.Context, s string) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (d testDatastore) GetFollowingByUsername(ctx context.Context, s string) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (d testDatastore) GetActorInbox(ctx context.Context, s string, s2 string, s3 string, b bool) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (d testDatastore) GetActorInboxAsOrderedCollection(ctx context.Context, s string, b bool) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (d testDatastore) GetPublicInbox(ctx context.Context, s string, s2 string, b bool, b2 bool) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (d testDatastore) GetPublicInboxAsOrderedCollection(ctx context.Context, b bool, b2 bool) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (d testDatastore) GetActorOutbox(ctx context.Context, s string, s2 string, s3 string) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (d testDatastore) GetActorOutboxAsOrderedCollection(ctx context.Context, s string) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (d testDatastore) GetLikedAsOrderedCollection(ctx context.Context, s string) (vocab.ActivityStreamsOrderedCollection, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (d testDatastore) GetLikeStatus(ctx context.Context, url *url.URL, url2 *url.URL) (bool, error) {
	return false, fmt.Errorf("unimplemented")
}

func (d testDatastore) UpdateActor(ctx context.Context, s string, s2 string, s3 string, image vocab.ActivityStreamsImage) error {
	return fmt.Errorf("unimplemented")
}

func newTestDatastore() *testDatastore {
	return &testDatastore{
		Actors: map[string]vocab.ActivityStreamsPerson{
			"brandonstark": generateTestPerson(),
		},
	}
}

type testClient struct {
	Client
}

func newTestClient() *testClient {
	return &testClient{}
}

func TestGetLocalPerson(t *testing.T) {
	tests := []struct {
		name       string
		username   string
		wantErr    bool
		wantPerson vocab.ActivityStreamsPerson
	}{
		{
			name:       "Test get local person brandonstark",
			username:   "brandonstark",
			wantPerson: generateTestPerson(),
			wantErr:    false,
		},
		{
			name:       "Test non-existent actor",
			username:   "fakeactor",
			wantPerson: nil,
			wantErr:    true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := NewServer(nil, newTestDatastore(), newTestClient(), nil)
			person, err := server.GetLocalPerson(context.Background(), test.username, false)
			if err != nil && !test.wantErr {
				t.Fatalf("GetLocalPerson() returned an unexpected error: got err=%v", err)
			}
			var gotPerson, wantPerson map[string]interface{}
			if person != nil {
				gotPerson, err = streams.Serialize(person)
				if err != nil && !test.wantErr {
					t.Fatalf("Failed to Serialize Got Person: got err=%v", err)
				}
			}
			if test.wantPerson != nil {
				wantPerson, err = streams.Serialize(test.wantPerson)
				if err != nil && !test.wantErr {
					t.Fatalf("Failed to Serialize Want Person: got err=%v", err)
				}
			}
			if d := cmp.Diff(wantPerson, gotPerson); d != "" {
				t.Errorf("ParseActor() returned an unexpected diff: (+got -want) %s", d)
			}
		})
	}
}
