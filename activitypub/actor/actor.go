package actor

import "fmt"

// Person is a type of Actor from https://www.w3.org/TR/activitypub/#actors.
type Person struct {
	Context []interface{} `json:"@context"`

	Type string `json:"type"`

	// Id is a URL to the Person's profile page.
	Id                string `json:"id"`
	PreferredUsername string `json:"preferredUsername"`
	Inbox             string `json:"inbox"`
	Outbox            string `json:"outbox"`

	Following string `json:"following"`
	Followers string `json:"followers"`
	Liked     string `json:"liked"`

	Icon string `json:"icon"`
	// Name is the display name of the Person.
	Name    string `json:"name"`
	Summary string `json:"summary"`

	PublicKey struct {
		Id           string `json:"id"`
		Owner        string `json:"owner"`
		PublicKeyPem string `json:"publicKeyPem"`
	} `json:"publicKey"`
}

// NewPerson initializes a new Actor of type Person.
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
	person.Context = []interface{}{
		"https://www.w3.org/ns/activitystreams",
	}
	person.Type = "Person"
	person.Id = fmt.Sprintf("%s/actor/%s", baseURL, username)
	person.Inbox = fmt.Sprintf("%s/actor/%s/inbox", baseURL, username)
	person.Outbox = fmt.Sprintf("%s/actor/%s/outbox", baseURL, username)
	person.Following = fmt.Sprintf("%s/actor/%s/following", baseURL, username)
	person.Followers = fmt.Sprintf("%s/actor/%s/followers", baseURL, username)
	person.Liked = fmt.Sprintf("%s/actor/%s/liked", baseURL, username)
	return person, nil
}
