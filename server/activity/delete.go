package activity

import (
	"context"
	"fmt"

	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	log "github.com/golang/glog"
)

func ParseDeleteActivity(ctx context.Context, activity vocab.Type) (vocab.ActivityStreamsDelete, error) {
	var delete vocab.ActivityStreamsDelete
	deleteResolver, err := streams.NewTypeResolver(func(ctx context.Context, d vocab.ActivityStreamsDelete) error {
		delete = d
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: got err=%v", err)
	}
	if err := deleteResolver.Resolve(ctx, activity); err != nil {
		return nil, fmt.Errorf("failed to resolve activity to Delete activity: got err=%v", err)
	}
	log.Infoln("Successfully resolved Type to ActivityStreamsDelete")
	return delete, nil
}
