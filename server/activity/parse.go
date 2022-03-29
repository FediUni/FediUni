package activity

import (
	"context"
	"encoding/json"
	"fmt"
	log "github.com/golang/glog"

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
	log.Infoln("Successfully resolved Type to Create Activity")
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
	log.Infoln("Successfully resolved Type to Announce Activity")
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
	log.Infoln("Successfully resolved Type to Like Activity")
	return like, err
}

func ParseFollowActivity(ctx context.Context, activity vocab.Type) (vocab.ActivityStreamsFollow, error) {
	if activity == nil {
		return nil, fmt.Errorf("failed to receive an Activity to parse: got=%v", activity)
	}
	var follow vocab.ActivityStreamsFollow
	followResolver, err := streams.NewTypeResolver(func(ctx context.Context, f vocab.ActivityStreamsFollow) error {
		follow = f
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create TypeResolver: got err=%v", err)
	}
	err = followResolver.Resolve(ctx, activity)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve type to Follow activity: got err=%v", err)
	}
	log.Infoln("Successfully resolved Type to Follow Activity")
	return follow, nil
}

func ParseAcceptActivity(ctx context.Context, activity vocab.Type) (vocab.ActivityStreamsAccept, error) {
	if activity == nil {
		return nil, fmt.Errorf("failed to receive an Activity to parse: got=%v", activity)
	}
	var accept vocab.ActivityStreamsAccept
	acceptResolver, err := streams.NewTypeResolver(func(ctx context.Context, a vocab.ActivityStreamsAccept) error {
		accept = a
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create TypeResolver: got err=%v", err)
	}
	err = acceptResolver.Resolve(ctx, activity)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve type to Follow activity: got err=%v", err)
	}
	log.Infoln("Successfully resolved Type to Accept Activity")
	return accept, nil
}

func ParseDeleteActivity(ctx context.Context, activity vocab.Type) (vocab.ActivityStreamsDelete, error) {
	if activity == nil {
		return nil, fmt.Errorf("failed to receive an Activity to parse: got=%v", activity)
	}
	var deleteActivity vocab.ActivityStreamsDelete
	deleteResolver, err := streams.NewTypeResolver(func(ctx context.Context, d vocab.ActivityStreamsDelete) error {
		deleteActivity = d
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: got err=%v", err)
	}
	if err := deleteResolver.Resolve(ctx, activity); err != nil {
		return nil, fmt.Errorf("failed to resolve activity to Delete activity: got err=%v", err)
	}
	log.Infoln("Successfully resolved Type to Delete Activity")
	return deleteActivity, nil
}

func ParseUndoActivity(ctx context.Context, activity vocab.Type) (vocab.ActivityStreamsUndo, error) {
	if activity == nil {
		return nil, fmt.Errorf("failed to receive an Activity to parse: got=%v", activity)
	}
	var undo vocab.ActivityStreamsUndo
	undoResolver, err := streams.NewTypeResolver(func(ctx context.Context, u vocab.ActivityStreamsUndo) error {
		undo = u
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: got err=%v", err)
	}
	if err := undoResolver.Resolve(ctx, activity); err != nil {
		return nil, fmt.Errorf("failed to resolve activity to Undo activity: got err=%v", err)
	}
	log.Infoln("Successfully resolved Type to Undo Activity")
	return undo, nil
}
