package activity

import (
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
