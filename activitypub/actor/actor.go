package actor

// Person is a type of Actor defined by https://www.w3.org/TR/activitypub/#actors.
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
