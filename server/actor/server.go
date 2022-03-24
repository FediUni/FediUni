package actor

import (
	"context"
	"fmt"
	"github.com/FediUni/FediUni/server/file"
	"github.com/FediUni/FediUni/server/object"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	log "github.com/golang/glog"
	"golang.org/x/sync/errgroup"
	"mime/multipart"
	"net/url"
	"path/filepath"
	"strings"
)

const (
	maxProfileImageSize = 5242880 // This represents a value of 5 MiB.
)

type Server struct {
	URL         *url.URL
	Client      Client
	Datastore   Datastore
	FileHandler *file.Handler
}

type Datastore interface {
	GetActorByUsername(context.Context, string) (vocab.ActivityStreamsPerson, error)
	GetFollowersByUsername(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error)
	GetFollowingByUsername(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error)
	GetActorInbox(context.Context, string, string, string, bool) (vocab.ActivityStreamsOrderedCollectionPage, error)
	GetActorInboxAsOrderedCollection(context.Context, string, bool) (vocab.ActivityStreamsOrderedCollection, error)
	GetPublicInbox(context.Context, string, string, bool, bool) (vocab.ActivityStreamsOrderedCollectionPage, error)
	GetPublicInboxAsOrderedCollection(context.Context, bool, bool) (vocab.ActivityStreamsOrderedCollection, error)
	GetActorOutbox(context.Context, string, string, string) (vocab.ActivityStreamsOrderedCollectionPage, error)
	GetActorOutboxAsOrderedCollection(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error)
	GetLikedAsOrderedCollection(context.Context, string) (vocab.ActivityStreamsOrderedCollection, error)
	UpdateActor(context.Context, string, string, string, vocab.ActivityStreamsImage) error
}
type Client interface {
	FetchRemoteActor(context.Context, string) (Actor, error)
	FetchRemoteObject(context.Context, *url.URL, bool, int, int) (vocab.Type, error)
	DereferenceFollowers(context.Context, vocab.ActivityStreamsFollowersProperty, int, int) error
	DereferenceFollowing(context.Context, vocab.ActivityStreamsFollowingProperty, int, int) error
	DereferenceOutbox(context.Context, vocab.ActivityStreamsOutboxProperty, int, int) error
	DereferenceObjectsInOrderedCollection(context.Context, vocab.ActivityStreamsOrderedCollection, int, int, int) (vocab.ActivityStreamsOrderedCollectionPage, error)
	DereferenceOrderedItems(context.Context, vocab.ActivityStreamsOrderedItemsProperty, int, int) error
}

// NewServer initializes a Server with methods for fetching actors and details.
func NewServer(url *url.URL, datastore Datastore, client Client, fileHandler *file.Handler) *Server {
	return &Server{
		URL:         url,
		Datastore:   datastore,
		Client:      client,
		FileHandler: fileHandler,
	}
}

// GetLocalPerson returns the ActivityPub actor with the corresponding username.
// If the statistics parameter is true then the number of Followers, Actors
// Followed, and the amount of Activities in the Outbox is returned.
func (s *Server) GetLocalPerson(ctx context.Context, username string, statistics bool) (vocab.Type, error) {
	person, err := s.Datastore.GetActorByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor with ID=%q: got err=%v", username, err)
	}
	var eg errgroup.Group
	if statistics {
		eg.Go(func() error { return s.Client.DereferenceFollowers(ctx, person.GetActivityStreamsFollowers(), 0, 1) })
		eg.Go(func() error { return s.Client.DereferenceFollowing(ctx, person.GetActivityStreamsFollowing(), 0, 1) })
		eg.Go(func() error { return s.Client.DereferenceOutbox(ctx, person.GetActivityStreamsOutbox(), 0, 1) })
		if err := eg.Wait(); err != nil {
			log.Errorf("Dereferencing has failed: got err=%v\nContinuing...", err)
		}
	}
	return person, nil
}

func (s *Server) GetAny(ctx context.Context, identifier string, statistics bool) (Actor, error) {
	actor, err := s.Client.FetchRemoteActor(ctx, identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve actor: got err=%v", err)
	}
	var eg errgroup.Group
	if statistics {
		eg.Go(func() error { return s.Client.DereferenceFollowers(ctx, actor.GetActivityStreamsFollowers(), 0, 1) })
		eg.Go(func() error { return s.Client.DereferenceFollowing(ctx, actor.GetActivityStreamsFollowing(), 0, 1) })
		eg.Go(func() error { return s.Client.DereferenceOutbox(ctx, actor.GetActivityStreamsOutbox(), 0, 1) })
		if err := eg.Wait(); err != nil {
			log.Errorf("Dereferencing has failed: got err=%v", err)
		}
	}
	return actor, nil
}

// UpdateLocal updates the internal representation of the actor.
// The mutable properties are the "name", "summary", and "icon".
// Handles directory creation and file naming.
func (s *Server) UpdateLocal(ctx context.Context, username, displayName, summary string, file multipart.File, header *multipart.FileHeader) error {
	if file != nil && header.Size > maxProfileImageSize {
		return fmt.Errorf("failed to parse profile picture: got size=%d and max size=%d", header.Size, maxProfileImageSize)
	}
	var image vocab.ActivityStreamsImage
	if file != nil {
		path, err := s.FileHandler.StoreProfilePicture(file, header.Filename)
		if err != nil {
			return fmt.Errorf("failed to store profile picture: got err=%v", err)
		}
		profilePictureURL, err := url.Parse(fmt.Sprintf("%s/static/%s", s.URL.String(), path))
		if err != nil {
			return fmt.Errorf("failed to create profile picture path: got err=%v", err)
		}
		image = streams.NewActivityStreamsImage()
		urlProperty := streams.NewActivityStreamsUrlProperty()
		image.SetActivityStreamsUrl(urlProperty)
		urlProperty.AppendIRI(profilePictureURL)
		mediaType := streams.NewActivityStreamsMediaTypeProperty()
		mediaType.Set(fmt.Sprintf("image/%s", strings.ToLower(filepath.Ext(header.Filename)[1:])))
		image.SetActivityStreamsMediaType(mediaType)
	}
	return s.Datastore.UpdateActor(ctx, username, displayName, summary, image)
}

// GetFollowers returns an OrderedCollection where items are Actor IRIs.
func (s *Server) GetFollowers(ctx context.Context, username string) (vocab.ActivityStreamsOrderedCollection, error) {
	return s.Datastore.GetFollowersByUsername(ctx, username)
}

// GetFollowing returns an OrderedCollection where items are Actor IRIs.
func (s *Server) GetFollowing(ctx context.Context, username string) (vocab.ActivityStreamsOrderedCollection, error) {
	return s.Datastore.GetFollowingByUsername(ctx, username)
}

// GetLikedAsOrderedCollection returns an OrderedCollection of Like IRIs.
func (s *Server) GetLikedAsOrderedCollection(ctx context.Context, username string) (vocab.ActivityStreamsOrderedCollection, error) {
	return s.Datastore.GetLikedAsOrderedCollection(ctx, username)
}

// GetInboxAsOrderedCollection returns an OrderedCollection with page IRIs.
func (s *Server) GetInboxAsOrderedCollection(ctx context.Context, username string, local bool) (vocab.ActivityStreamsOrderedCollection, error) {
	return s.Datastore.GetActorInboxAsOrderedCollection(ctx, username, local)
}

// GetInboxPage returns an OrderedCollectionPage with dereferenced objects.
func (s *Server) GetInboxPage(ctx context.Context, username, minID, maxID string, local bool) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	page, err := s.Datastore.GetActorInbox(ctx, username, minID, maxID, local)
	if err != nil {
		return nil, fmt.Errorf("failed to read from Inbox of Username=%q: got err=%v", username, err)
	}
	orderedItems := page.GetActivityStreamsOrderedItems()
	if orderedItems == nil {
		return page, nil
	}
	if err := s.Client.DereferenceOrderedItems(ctx, orderedItems, 0, 3); err != nil {
		return nil, fmt.Errorf("failed to dereference orderedItems in OrderedCollection: got err=%v", err)
	}
	return page, nil
}

// GetPublicInboxAsOrderedCollection returns an OrderedCollection with IRIs.
func (s *Server) GetPublicInboxAsOrderedCollection(ctx context.Context, local, institute bool) (vocab.ActivityStreamsOrderedCollection, error) {
	return s.Datastore.GetPublicInboxAsOrderedCollection(ctx, local, institute)
}

// GetPublicInboxPage returns an OrderedCollectionPage with objects.
func (s *Server) GetPublicInboxPage(ctx context.Context, minID, maxID string, local, institute bool) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	page, err := s.Datastore.GetPublicInbox(ctx, minID, maxID, local, institute)
	if err != nil {
		return nil, fmt.Errorf("failed to read from Public Inbox: got err=%v", err)
	}
	orderedItems := page.GetActivityStreamsOrderedItems()
	if orderedItems == nil {
		return page, nil
	}
	if err := s.Client.DereferenceOrderedItems(ctx, orderedItems, 0, 3); err != nil {
		return nil, fmt.Errorf("failed to dereference orderedItems in OrderedCollection: got err=%v", err)
	}
	return page, nil
}

// GetOutbox returns an OrderedCollection with IRIs.
func (s *Server) GetOutbox(ctx context.Context, username string) (vocab.ActivityStreamsOrderedCollection, error) {
	return s.Datastore.GetActorOutboxAsOrderedCollection(ctx, username)
}

// GetOutboxPage returns an OrderedCollectionPage with objects.
func (s *Server) GetOutboxPage(ctx context.Context, username, minID, maxID string) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	page, err := s.Datastore.GetActorOutbox(ctx, username, minID, maxID)
	if err != nil {
		return nil, fmt.Errorf("failed to read from Inbox of Username=%q: got err=%v", username, err)
	}
	orderedItems := page.GetActivityStreamsOrderedItems()
	if orderedItems == nil {
		return page, nil
	}
	if err := s.Client.DereferenceOrderedItems(ctx, orderedItems, 0, 3); err != nil {
		return nil, fmt.Errorf("failed to dereference orderedItems in OrderedCollection: got err=%v", err)
	}
	return page, nil
}

// GetAnyOutbox fetches and dereferences any local or remote actor's outbox.
// The dereference page in the OrderedCollection is returned.
func (s *Server) GetAnyOutbox(ctx context.Context, identifier string, page int) (vocab.ActivityStreamsOrderedCollectionPage, error) {
	person, err := s.Client.FetchRemoteActor(ctx, identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch a remote Actor=%q: got err=%v", identifier, err)
	}
	personID := person.GetJSONLDId().Get()
	log.Infof("Loading Outbox of Actor ID=%q", person.GetJSONLDId().Get().String())
	outbox := person.GetActivityStreamsOutbox()
	if outbox == nil {
		return nil, fmt.Errorf("failed to load Outbox: got=%v", outbox)
	}
	outboxIRI := outbox.GetIRI()
	if outboxIRI == nil {
		return nil, fmt.Errorf("failed to load Outbox URL: got=%v", outboxIRI)
	}
	log.Infof("Fetching Outbox Collection of Actor ID=%q", personID.String())
	o, err := s.Client.FetchRemoteObject(ctx, outboxIRI, false, 0, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Outbox Collection: got err=%v", err)
	}
	orderedCollection, err := object.ParseOrderedCollection(ctx, o)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OrderedCollection from Outbox: got err=%v", err)
	}
	log.Infof("Dereferencing Outbox Collection ID=%q of actor ID=%q", outboxIRI.String(), personID.String())
	return s.Client.DereferenceObjectsInOrderedCollection(ctx, orderedCollection, page, 0, 3)
}
