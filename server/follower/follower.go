package follower

import (
	"net/url"

	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
)

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
