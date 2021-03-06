package validation

import (
	"context"
	"encoding/json"
	"github.com/FediUni/FediUni/server/actor"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	"github.com/go-fed/httpsig"
	"io/ioutil"
	"net/http"

	log "github.com/golang/glog"
)

// Signature validates the "Signature" header using the public key.
func Signature(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifier, err := httpsig.NewVerifier(r)
		if err != nil {
			log.Errorf("failed to create verifier: got err=%v", err)
			http.Error(w, "failed to validate signature", http.StatusInternalServerError)
			return
		}
		keyID := verifier.KeyId()
		req, err := http.NewRequest(http.MethodGet, keyID, nil)
		if err != nil {
			log.Errorf("failed to create new request: got err=%v", err)
			http.Error(w, "failed to retrieve public key", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Accept", "application/activity+json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Errorf("failed to retrieve public key, got err=%v", err)
			http.Error(w, "failed to retrieve public key", http.StatusInternalServerError)
			return
		}
		defer res.Body.Close()
		marshalledActor, err := ioutil.ReadAll(res.Body)
		if err != nil {
			log.Errorf("failed to read from body, got err=%v", err)
			http.Error(w, "failed to retrieve public key", http.StatusInternalServerError)
			return
		}
		var person actor.Person
		resolver, err := streams.NewJSONResolver(func(c context.Context, p vocab.ActivityStreamsPerson) error {
			person = p
			return nil
		})
		var m map[string]interface{}
		if err := json.Unmarshal(marshalledActor, &m); err != nil {
			log.Errorf("failed to unmarshal person, got err=%v", err)
			http.Error(w, "failed to retrieve public key", http.StatusInternalServerError)
			return
		}
		if err := resolver.Resolve(r.Context(), m); err != nil {
			log.Errorf("failed to resolve JSON, got err=%v", err)
			http.Error(w, "failed to retrieve public key", http.StatusInternalServerError)
			return
		}
		if err != nil {
			log.Errorf("failed to create resolver, got err=%v", err)
			http.Error(w, "failed to retrieve public key", http.StatusInternalServerError)
			return
		}
		publicKeyProperty := person.GetW3IDSecurityV1PublicKey()
		if publicKeyProperty == nil {
			http.Error(w, "Failed to retrieve public key", http.StatusBadRequest)
			return
		}
		var publicKey vocab.W3IDSecurityV1PublicKey
		for iter := publicKeyProperty.Begin(); iter != publicKeyProperty.End(); iter = iter.Next() {
			if iter.IsW3IDSecurityV1PublicKey() {
				publicKey = iter.Get()
				break
			}
		}
		parsedKey, err := parsePublicKeyFromPEMBlock(publicKey.GetW3IDSecurityV1PublicKeyPem().Get())
		if err != nil {
			log.Errorf("failed to parse public key from block, got err=%v", err)
			http.Error(w, "failed to validate signature", http.StatusInternalServerError)
			return
		}
		log.Infoln("Public Key successfully parsed.")
		if err := verifier.Verify(parsedKey, httpsig.RSA_SHA256); err != nil {
			log.Errorf("failed to verify the provided signature, got err=%v", err)
			http.Error(w, "failed to validate signature", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r)
	})
}
