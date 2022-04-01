package actor

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
)

type Actor interface {
	vocab.Type
	GetActivityStreamsFollowers() vocab.ActivityStreamsFollowersProperty
	GetActivityStreamsFollowing() vocab.ActivityStreamsFollowingProperty
	GetActivityStreamsInbox() vocab.ActivityStreamsInboxProperty
	GetActivityStreamsOutbox() vocab.ActivityStreamsOutboxProperty
}

// Person is a type of Actor from https://www.w3.org/TR/activitypub/#actors.
type Person vocab.ActivityStreamsPerson

type PublicKey vocab.W3IDSecurityV1PublicKey

type RSAKeyGenerator struct {
	PrivateKey *bytes.Buffer
	PublicKey  *bytes.Buffer
}

func NewRSAKeyGenerator() *RSAKeyGenerator {
	return &RSAKeyGenerator{
		PrivateKey: &bytes.Buffer{},
		PublicKey:  &bytes.Buffer{},
	}
}

type KeyGenerator interface {
	GenerateKeyPair() (string, string, error)
	WritePrivateKey(string) error
	GetPrivateKeyPEM() ([]byte, error)
}

type PersonGenerator struct {
	InstanceURL  *url.URL
	KeyGenerator KeyGenerator
}

func NewPersonGenerator(url *url.URL, keyGenerator KeyGenerator) *PersonGenerator {
	return &PersonGenerator{
		InstanceURL:  url,
		KeyGenerator: keyGenerator,
	}
}

func (p *PersonGenerator) NewPerson(ctx context.Context, username, displayName string) (Person, error) {
	if username == "" {
		return nil, fmt.Errorf("failed to receive a username: got=%q", username)
	}
	serializedPerson := map[string]interface{}{
		"@context":          "https://www.w3.org/ns/activitystreams",
		"type":              "Person",
		"id":                fmt.Sprintf("%s/actor/%s", p.InstanceURL.String(), username),
		"preferredUsername": username,
		"inbox":             fmt.Sprintf("%s/actor/%s/inbox", p.InstanceURL.String(), username),
		"outbox":            fmt.Sprintf("%s/actor/%s/outbox", p.InstanceURL.String(), username),
		"followers":         fmt.Sprintf("%s/actor/%s/followers", p.InstanceURL.String(), username),
		"following":         fmt.Sprintf("%s/actor/%s/following", p.InstanceURL.String(), username),
		"liked":             fmt.Sprintf("%s/actor/%s/liked", p.InstanceURL.String(), username),
	}
	if displayName != "" {
		serializedPerson["name"] = displayName
	}
	actor, err := streams.ToType(ctx, serializedPerson)
	if err != nil {
		return nil, fmt.Errorf("failed to convert JSON: got err=%v", err)
	}
	person, err := ParsePerson(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Actor: got err=%v", err)
	}
	publicKeyProperty := streams.NewW3IDSecurityV1PublicKeyProperty()
	publicKey, err := p.GeneratePublicKey(username)
	if err != nil {
		return nil, err
	}
	publicKeyProperty.AppendW3IDSecurityV1PublicKey(publicKey)
	person.SetW3IDSecurityV1PublicKey(publicKeyProperty)
	return person, nil
}

func (p *PersonGenerator) GeneratePublicKey(username string) (vocab.W3IDSecurityV1PublicKey, error) {
	_, publicKeyPEM, err := p.KeyGenerator.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate public key: got err=%v", err)
	}
	publicKey := streams.NewW3IDSecurityV1PublicKey()
	publicKeyURL, err := url.Parse(fmt.Sprintf("%s/actor/%s#main-key", p.InstanceURL.String(), username))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Public Key ID for Person: got err=%v", err)
	}
	publicKeyID := streams.NewJSONLDIdProperty()
	publicKeyID.Set(publicKeyURL)
	publicKey.SetJSONLDId(publicKeyID)
	pemProperty := streams.NewW3IDSecurityV1PublicKeyPemProperty()
	pemProperty.Set(publicKeyPEM)
	publicKey.SetW3IDSecurityV1PublicKeyPem(pemProperty)
	return publicKey, nil
}

func (g *RSAKeyGenerator) GenerateKeyPair() (string, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: got err=%v", err)
	}
	if err = pem.Encode(g.PrivateKey, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}); err != nil {
		return "", "", fmt.Errorf("failed to encode RSA private key in PEM format: got err=%v", err)
	}
	marshalledPublicKey, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal PKIX public key: got err=%v", err)
	}
	if err = pem.Encode(g.PublicKey, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: marshalledPublicKey,
	}); err != nil {
		return "", "", fmt.Errorf("failed to encode public key in PEM format: got err=%v", err)
	}
	return g.PrivateKey.String(), g.PublicKey.String(), nil
}

func (g *RSAKeyGenerator) GetPrivateKeyPEM() ([]byte, error) {
	if g.PrivateKey.Len() == 0 {
		return nil, fmt.Errorf("private key has not been generated")
	}
	return g.PrivateKey.Bytes(), nil
}

func (g *RSAKeyGenerator) WritePrivateKey(privateKeyPath string) error {
	privatePem, err := os.Create(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to create private pem file: got err=%v", err)
	}
	defer privatePem.Close()
	if _, err := g.PrivateKey.WriteTo(privatePem); err != nil {
		return fmt.Errorf("failed to write private key to path=%q: got err=%v", privateKeyPath, err)
	}
	return nil
}

// IsIdentifier is of the form @username@domain then this function returns true.
func IsIdentifier(identifier string) (bool, error) {
	if identifier == "" {
		return false, fmt.Errorf("identifier must not be empty: got=%q", identifier)
	}
	splitIdentifier := strings.Split(identifier, "@")
	if len(splitIdentifier) == 3 {
		return true, nil
	}
	return false, nil
}

func ParsePerson(ctx context.Context, actor vocab.Type) (vocab.ActivityStreamsPerson, error) {
	var person vocab.ActivityStreamsPerson
	resolver, err := streams.NewTypeResolver(func(ctx context.Context, p vocab.ActivityStreamsPerson) error {
		person = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := resolver.Resolve(ctx, actor); err != nil {
		return nil, err
	}
	return person, nil
}

// ParseActor parses an Actor from the ActivityPub Object provided.
// Returns an Actor if it is a Person, Service, Group, Organization or
// Application.
func ParseActor(ctx context.Context, rawActor vocab.Type) (Actor, error) {
	if rawActor == nil {
		return nil, fmt.Errorf("failed to receive actor: got=%v", rawActor)
	}
	var actor Actor
	actorResolver, err := streams.NewTypeResolver(func(ctx context.Context, a vocab.ActivityStreamsPerson) error {
		actor = a
		return nil
	}, func(ctx context.Context, a vocab.ActivityStreamsService) error {
		actor = a
		return nil
	}, func(ctx context.Context, a vocab.ActivityStreamsGroup) error {
		actor = a
		return nil
	}, func(ctx context.Context, a vocab.ActivityStreamsOrganization) error {
		actor = a
		return nil
	}, func(ctx context.Context, a vocab.ActivityStreamsApplication) error {
		actor = a
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := actorResolver.Resolve(ctx, rawActor); err != nil {
		return nil, err
	}
	return actor, nil
}
