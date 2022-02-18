package undo

import (
	"context"
	"fmt"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	log "github.com/golang/glog"
)

func ParseUndoRequest(ctx context.Context, activity vocab.Type) (vocab.ActivityStreamsUndo, error) {
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
	log.Infoln("Successfully resolved Type to ActivityStreamsUndo")
	return undo, nil
}
