package actor

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

type PublicKey struct {
	Id           string `json:"id"`
	Owner        string `json:"owner"`
	PublicKeyPem string `json:"publicKeyPem"`
}

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

	PublicKey *PublicKey
}

// NewPerson initializes a new Actor of type Person.
func NewPerson(username, displayName, baseURL, keyDirectory string) (*Person, error) {
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
	publicKey, err := generateKeyPair(username, keyDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to generate public key: got err=%v", err)
	}
	person.PublicKey = &PublicKey{
		Id:           fmt.Sprintf("%s/actor/%s#public-key", baseURL, username),
		Owner:        person.Id,
		PublicKeyPem: string(publicKey),
	}
	return person, nil
}

func generateKeyPair(username, keyDirectory string) ([]byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: got err=%v", err)
	}
	privatePem, err := os.Create(filepath.Join(keyDirectory, fmt.Sprintf("%s_private.pem", username)))
	if err != nil {
		return nil, fmt.Errorf("failed to create private pem file: got err=%v", err)
	}
	if err = pem.Encode(privatePem, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}); err != nil {
		return nil, fmt.Errorf("failed to encode RSA private key in PEM format: got err=%v", err)
	}
	publicPem := &bytes.Buffer{}
	if err = pem.Encode(publicPem, &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	}); err != nil {
		return nil, fmt.Errorf("failed to encode RSA public key in PEM format: got err=%v", err)
	}
	return publicPem.Bytes(), nil
}
