package actor

import "fmt"

// Person is a type of Actor from https://www.w3.org/TR/activitypub/#actors.
type Person struct {
	Context []interface{} `json:"@context"`

	Type string `json:"type"`

	Id                string `json:"id"`
	PreferredUsername string `json:"preferredUsername"`
	Inbox             string `json:"inbox"`
	Outbox            string `json:"outbox"`

	Following string `json:"following"`
	Followers string `json:"followers"`
	Liked     string `json:"liked"`

	Icon    string `json:"icon"`
	Name    string `json:"name"`
	Summary string `json:"summary"`

	PublicKey struct {
		Id           string `json:"id"`
		Owner        string `json:"owner"`
		PublicKeyPem string `json:"publicKeyPem"`
	} `json:"publicKey"`
}

func NewPerson(username, displayName, baseURL string) (*Person, error) {
	person := &Person{}
	if username == "" {
		return nil, fmt.Errorf("username must not be %q", username)
	}
	person.PreferredUsername = username
	person.Name = username
	if displayName != "" {
		person.Name = displayName
	}
	person.Inbox = fmt.Sprintf("%s/actor/%s/inbox", baseURL, username)
	person.Outbox = fmt.Sprintf("%s/actor/%s/outbox", baseURL, username)
	person.Following = fmt.Sprintf("%s/actor/%s/followering", baseURL, username)
	person.Followers = fmt.Sprintf("%s/actor/%s/followers", baseURL, username)
	return person, nil
}
