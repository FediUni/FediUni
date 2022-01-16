package activity

import (
	"encoding/json"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"io/ioutil"
	"net/http"
	"time"
)

type Activity struct {
	Context   interface{}        `json:"@context"`
	MongoID   primitive.ObjectID `bson:"_id"`
	ID        string             `json:"id"`
	Type      string             `json:"type"`
	Published time.Time          `json:"time"`
	Actor     string             `json:"actor"`
	To        []string           `json:"to"`
	CC        []string           `json:"cc"`
	Object    interface {
	} `json:"object"`
}

// ParseActivity returns an Activity from an HTTP Request.
func ParseActivity(r *http.Request) (*Activity, error) {
	raw, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read from request body: got err=%v", err)
	}
	var activity *Activity
	if err := json.Unmarshal(raw, &activity); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON from request body as Activity: got err=%v", err)
	}
	return activity, nil
}

func (a *Activity) BSON() ([]byte, error) {
	activity, err := bson.Marshal(a)
	if err != nil {
		return nil, err
	}
	return activity, nil
}
