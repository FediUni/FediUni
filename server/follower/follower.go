package follower

import (
	"context"
	"fmt"
	"net/url"

	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	log "github.com/golang/glog"
)

func ParseFollowRequest(ctx context.Context, activity vocab.Type) (vocab.ActivityStreamsFollow, error) {
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
	log.Infoln("Successfully resolved Type to ActivityStreamsFollow")
	return follow, nil
}

func ParseAcceptActivity(ctx context.Context, activity vocab.Type) (vocab.ActivityStreamsAccept, error) {
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
	log.Infoln("Successfully resolved Type to ActivityStreamsFollow")
	return accept, nil
}

func PrepareAcceptActivity(follow vocab.ActivityStreamsFollow, actorID *url.URL) vocab.ActivityStreamsAccept {
	acceptActivity := streams.NewActivityStreamsAccept()
	actorProperty := streams.NewActivityStreamsActorProperty()
	actorProperty.AppendIRI(actorID)
	acceptActivity.SetActivityStreamsActor(actorProperty)
	objectProperty := streams.NewActivityStreamsObjectProperty()
	objectProperty.AppendActivityStreamsFollow(follow)
	acceptActivity.SetActivityStreamsObject(objectProperty)
	return acceptActivity
}
