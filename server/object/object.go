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
