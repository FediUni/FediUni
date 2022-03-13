package object

import (
	"context"

	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
)

func ParseNote(ctx context.Context, object vocab.Type) (vocab.ActivityStreamsNote, error) {
	var note vocab.ActivityStreamsNote
	noteResolver, err := streams.NewTypeResolver(func(ctx context.Context, n vocab.ActivityStreamsNote) error {
		note = n
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := noteResolver.Resolve(ctx, object); err != nil {
		return nil, err
	}
	return note, nil
}

func ParseOrderedCollection(ctx context.Context, object vocab.Type) (vocab.ActivityStreamsOrderedCollection, error) {
	var orderedCollection vocab.ActivityStreamsOrderedCollection
	orderedCollectionResolver, err := streams.NewTypeResolver(func(ctx context.Context, o vocab.ActivityStreamsOrderedCollection) error {
		orderedCollection = o
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := orderedCollectionResolver.Resolve(ctx, object); err != nil {
		return nil, err
	}
	return orderedCollection, nil
}

func ParseCollection(ctx context.Context, object vocab.Type) (vocab.ActivityStreamsCollection, error) {
	var collection vocab.ActivityStreamsCollection
	collectionResolver, err := streams.NewTypeResolver(func(ctx context.Context, c vocab.ActivityStreamsCollection) error {
		collection = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := collectionResolver.Resolve(ctx, object); err != nil {
		return nil, err
	}
	return collection, nil
}
