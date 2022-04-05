package object

import (
	"context"
	"fmt"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	log "github.com/golang/glog"
	"net/url"
)

type Object interface {
	vocab.Type
	GetActivityStreamsPublished() vocab.ActivityStreamsPublishedProperty
	GetActivityStreamsTo() vocab.ActivityStreamsToProperty
	GetActivityStreamsCc() vocab.ActivityStreamsCcProperty
	GetActivityStreamsInReplyTo() vocab.ActivityStreamsInReplyToProperty
}

func ParseObject(ctx context.Context, rawObject vocab.Type) (Object, error) {
	var object Object
	objectResolver, err := streams.NewTypeResolver(func(ctx context.Context, o Object) error {
		object = o
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := objectResolver.Resolve(ctx, object); err != nil {
		return nil, err
	}
	return object, nil
}

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
	log.Infoln("Successfully parsed Note Object")
	return note, nil
}

func ParseEvent(ctx context.Context, object vocab.Type) (vocab.ActivityStreamsEvent, error) {
	var event vocab.ActivityStreamsEvent
	eventResolver, err := streams.NewTypeResolver(func(ctx context.Context, e vocab.ActivityStreamsEvent) error {
		event = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := eventResolver.Resolve(ctx, object); err != nil {
		return nil, err
	}
	return event, nil
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

// WrapInCreate accepts objects and wraps an object in a Create activity.
// WrapInCreate does not handle ID assignment.
func WrapInCreate(ctx context.Context, object Object, actor vocab.Type) (vocab.ActivityStreamsCreate, error) {
	if actor == nil {
		return nil, fmt.Errorf("failed to receive actor: got=%v", actor)
	}
	create := streams.NewActivityStreamsCreate()
	o := streams.NewActivityStreamsObjectProperty()
	create.SetActivityStreamsObject(o)
	if err := o.AppendType(object); err != nil {
		return nil, err
	}
	actorProperty := streams.NewActivityStreamsActorProperty()
	create.SetActivityStreamsActor(actorProperty)
	if err := actorProperty.AppendType(actor); err != nil {
		return nil, err
	}
	create.SetActivityStreamsPublished(object.GetActivityStreamsPublished())
	create.SetActivityStreamsTo(object.GetActivityStreamsTo())
	create.SetActivityStreamsCc(object.GetActivityStreamsCc())
	return create, nil
}

// WrapInInvite accepts an event and wraps the event in an Invite activity.
// WrapInInvite does not handle ID assignment or determining targets.
func WrapInInvite(event vocab.ActivityStreamsEvent, actor vocab.Type) (vocab.ActivityStreamsInvite, error) {
	if actor == nil {
		return nil, fmt.Errorf("failed to receive actor: got=%v", actor)
	}
	invite := streams.NewActivityStreamsInvite()
	o := streams.NewActivityStreamsObjectProperty()
	invite.SetActivityStreamsObject(o)
	if err := o.AppendType(event); err != nil {
		return nil, err
	}
	actorProperty := streams.NewActivityStreamsActorProperty()
	invite.SetActivityStreamsActor(actorProperty)
	if err := actorProperty.AppendType(actor); err != nil {
		return nil, err
	}
	invite.SetActivityStreamsPublished(event.GetActivityStreamsPublished())
	invite.SetActivityStreamsTo(event.GetActivityStreamsTo())
	invite.SetActivityStreamsCc(event.GetActivityStreamsCc())
	return invite, nil
}

// WrapInAccept accepts an activity ID and wraps it in an Accept activity.
// WrapInAccept does not handle ID assignment.
func WrapInAccept(activityID *url.URL, actorID *url.URL) (vocab.ActivityStreamsAccept, error) {
	if activityID == nil {
		return nil, fmt.Errorf("failed to append Object to Accept Activity: got Object ID=%v", activityID)
	}
	if actorID == nil {
		return nil, fmt.Errorf("failed to append Actor to Accept Activity: got Object ID=%v", actorID)
	}
	acceptActivity := streams.NewActivityStreamsAccept()
	actorProperty := streams.NewActivityStreamsActorProperty()
	actorProperty.AppendIRI(actorID)
	acceptActivity.SetActivityStreamsActor(actorProperty)
	objectProperty := streams.NewActivityStreamsObjectProperty()
	objectProperty.AppendIRI(activityID)
	acceptActivity.SetActivityStreamsObject(objectProperty)
	return acceptActivity, nil
}
