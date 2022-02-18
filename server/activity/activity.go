package activity

import (
	"context"
	"encoding/json"

	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
)

func JSON(activity vocab.Type) ([]byte, error) {
	m, err := streams.Serialize(activity)
	if err != nil {
		return nil, err
	}
	marshalledActivity, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return marshalledActivity, nil
}

func ParseCreateActivity(ctx context.Context, activity vocab.Type) (vocab.ActivityStreamsCreate, error) {
	var create vocab.ActivityStreamsCreate
	createResolver, err := streams.NewTypeResolver(func(ctx context.Context, c vocab.ActivityStreamsCreate) error {
		create = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := createResolver.Resolve(ctx, activity); err != nil {
		return nil, err
	}
	return create, nil
}
