package activity

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
)

type Activity interface {
	vocab.Type
	GetActivityStreamsTo() vocab.ActivityStreamsToProperty
	GetActivityStreamsCc() vocab.ActivityStreamsCcProperty
	GetActivityStreamsObject() vocab.ActivityStreamsObjectProperty
}

func JSON(activity vocab.Type) ([]byte, error) {
	if activity == nil {
		return nil, fmt.Errorf("failed to receive an Activity to marshal as JSON: got=%v", activity)
	}
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
	if activity == nil {
		return nil, fmt.Errorf("failed to receive an Activity to parse: got=%v", activity)
	}
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

func ParseAnnounceActivity(ctx context.Context, activity vocab.Type) (vocab.ActivityStreamsAnnounce, error) {
	if activity == nil {
		return nil, fmt.Errorf("failed to receive an Activity to parse: got=%v", activity)
	}
	var announce vocab.ActivityStreamsAnnounce
	announceResolver, err := streams.NewTypeResolver(func(ctx context.Context, a vocab.ActivityStreamsAnnounce) error {
		announce = a
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := announceResolver.Resolve(ctx, activity); err != nil {
		return nil, err
	}
	return announce, nil
}

func ParseLikeActivity(ctx context.Context, activity vocab.Type) (vocab.ActivityStreamsLike, error) {
	if activity == nil {
		return nil, fmt.Errorf("failed to receive an Activity to parse: got=%v", activity)
	}
	var like vocab.ActivityStreamsLike
	likeResolver, err := streams.NewTypeResolver(func(ctx context.Context, l vocab.ActivityStreamsLike) error {
		like = l
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := likeResolver.Resolve(ctx, activity); err != nil {
		return nil, err
	}
	return like, err
}
