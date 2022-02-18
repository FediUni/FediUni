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

func (p *PersonGenerator) NewPerson(username, displayName string) (Person, error) {
	person := streams.NewActivityStreamsPerson()
	contextProperty := streams.NewActivityStreamsContextProperty()
	activityStreamsURL, err := url.Parse("https://www.w3.org/ns/activitystreams")
	if err != nil {
		return nil, fmt.Errorf("failed to parse Context URL for Person: got err=%v", err)
	}
	contextProperty.AppendIRI(activityStreamsURL)
	person.SetActivityStreamsContext(contextProperty)
	personID := streams.NewJSONLDIdProperty()
	id, err := url.Parse(fmt.Sprintf("%s/actor/%s", p.InstanceURL.String(), username))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ID URL for Person: got err=%v", err)
	}
	personID.Set(id)
	person.SetJSONLDId(personID)
	inboxURL, err := url.Parse(fmt.Sprintf("%s/actor/%s/inbox", p.InstanceURL.String(), username))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Inbox URL for Person: got err=%v", err)
	}
	inbox := streams.NewActivityStreamsInboxProperty()
	inbox.SetIRI(inboxURL)
	person.SetActivityStreamsInbox(inbox)
	outboxURL, err := url.Parse(fmt.Sprintf("%s/actor/%s/outbox", p.InstanceURL.String(), username))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Outbox URL for Person: got err=%v", err)
	}
	outbox := streams.NewActivityStreamsOutboxProperty()
	outbox.SetIRI(outboxURL)
	person.SetActivityStreamsOutbox(outbox)
	followingURL, err := url.Parse(fmt.Sprintf("%s/actor/%s/following", p.InstanceURL.String(), username))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Following URL for Person: got err=%v", err)
	}
	following := streams.NewActivityStreamsFollowingProperty()
	following.SetIRI(followingURL)
	person.SetActivityStreamsFollowing(following)
	followersURL, err := url.Parse(fmt.Sprintf("%s/actor/%s/followers", p.InstanceURL.String(), username))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Followers URL for Person: got err=%v", err)
	}
	followers := streams.NewActivityStreamsFollowersProperty()
	followers.SetIRI(followersURL)
	person.SetActivityStreamsFollowers(followers)
	likedURL, err := url.Parse(fmt.Sprintf("%s/actor/%s/liked", p.InstanceURL.String(), username))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Liked URL for Person: got err=%v", err)
	}
	liked := streams.NewActivityStreamsLikedProperty()
	liked.SetIRI(likedURL)
	person.SetActivityStreamsLiked(liked)
	publicKeyProperty := streams.NewW3IDSecurityV1PublicKeyProperty()
	publicKey, err := p.GeneratePublicKey(username)
	if err != nil {
		return nil, err
	}
	publicKeyProperty.AppendW3IDSecurityV1PublicKey(publicKey)
	person.SetW3IDSecurityV1PublicKey(publicKeyProperty)
	preferredUsernameProperty := streams.NewActivityStreamsPreferredUsernameProperty()
	preferredUsernameProperty.SetXMLSchemaString(strings.ToLower(displayName))
	person.SetActivityStreamsPreferredUsername(preferredUsernameProperty)
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
	createResolver, err := streams.NewTypeResolver(func(ctx context.Context, p vocab.ActivityStreamsPerson) error {
		person = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := createResolver.Resolve(ctx, actor); err != nil {
		return nil, err
	}
	return person, nil
}
