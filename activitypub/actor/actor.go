package actor

import (
	"bytes"
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

type PKCS1KeyGenerator struct {
	PrivateKeyPEM *bytes.Buffer
	PublicKeyPEM  *bytes.Buffer
}

func NewPKCS1KeyGenerator() *PKCS1KeyGenerator {
	return &PKCS1KeyGenerator{
		PrivateKeyPEM: &bytes.Buffer{},
		PublicKeyPEM:  &bytes.Buffer{},
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

func (g *PKCS1KeyGenerator) GenerateKeyPair() (string, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: got err=%v", err)
	}
	if err = pem.Encode(g.PrivateKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}); err != nil {
		return "", "", fmt.Errorf("failed to encode RSA private key in PEM format: got err=%v", err)
	}
	if err = pem.Encode(g.PublicKeyPEM, &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	}); err != nil {
		return "", "", fmt.Errorf("failed to encode RSA public key in PEM format: got err=%v", err)
	}
	return g.PrivateKeyPEM.String(), g.PublicKeyPEM.String(), nil
}

func (g *PKCS1KeyGenerator) GetPrivateKeyPEM() ([]byte, error) {
	if g.PrivateKeyPEM.Len() == 0 {
		return nil, fmt.Errorf("private key has not been generated")
	}
	return g.PrivateKeyPEM.Bytes(), nil
}

func (g *PKCS1KeyGenerator) WritePrivateKey(privateKeyPath string) error {
	privatePem, err := os.Create(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to create private pem file: got err=%v", err)
	}
	defer privatePem.Close()
	if _, err := g.PrivateKeyPEM.WriteTo(privatePem); err != nil {
		return fmt.Errorf("failed to write private key to path=%q: got err=%v", privateKeyPath, err)
	}
	return nil
}
