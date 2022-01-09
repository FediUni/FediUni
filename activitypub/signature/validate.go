package signature

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/FediUni/FediUni/activitypub/actor"
	log "github.com/golang/glog"
	"io/ioutil"
	"net/http"
	"strings"
)

// Validate validates the "Signature" header using the public key.
func Validate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signatureHeader, err := processSignatureHeader(r.Header)
		if err != nil {
			log.Errorf("failed to parse signature, got err=%v", err)
			http.Error(w, "failed to parse signature", http.StatusBadRequest)
			return
		}
		if signatureHeader["signature"] == "" {
			log.Errorf("signature not provided")
			http.Error(w, "signature not provided", http.StatusBadRequest)
			return
		}
		signature, err := base64.StdEncoding.DecodeString(signatureHeader["signature"])
		if err != nil {
			log.Errorf("failed to decode signature, got err=%v", err)
			http.Error(w, "failed to parse signature", http.StatusBadRequest)
			return
		}
		keyID := signatureHeader["keyId"]
		if keyID == "" {
			log.Errorf("keyId not provided")
			http.Error(w, "keyId not provided", http.StatusBadRequest)
			return
		}
		log.Infof("Retrieving public key from url=%q", keyID)
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
		person := &actor.Person{}
		if err := json.Unmarshal(marshalledActor, &person); err != nil || person == nil {
			log.Errorf("failed to unmarshal person, got err=%v", err)
			http.Error(w, "failed to retrieve public key", http.StatusInternalServerError)
			return
		}
		if person.PublicKey == nil {
			log.Errorf("actor does not contain a public key")
			http.Error(w, "failed to retrieve a public key", http.StatusInternalServerError)
			return
		}
		pairs := []string{}
		for _, header := range strings.Split(signatureHeader["headers"], " ") {
			var pair string
			switch headerName := strings.ToLower(header); headerName {
			// (request-target) is a fake header that must be constructed using
			// HTTP method and the path.
			case "(request-target)":
				pair = fmt.Sprintf("%s: %s %s", headerName, strings.ToLower(r.Method), r.URL.Path)
			// Host header is removed from incoming requests and promoted to a
			// field (See: https://pkg.go.dev/net/http#Request).
			case "host":
				pair = fmt.Sprintf("%s: %s", headerName, r.Host)
			default:
				pair = fmt.Sprintf("%s: %s", headerName, r.Header.Get(header))
			}
			pairs = append(pairs, pair)
		}
		toCompare := strings.Join(pairs, "\n")
		log.Infoln("Parsing Public Key from PEM block...")
		publicKey, err := parsePublicKeyFromPEMBlock(person.PublicKey.PublicKeyPem)
		if err != nil {
			log.Errorf("failed to parse public key from block, got err=%v", err)
			http.Error(w, "failed to validate signature", http.StatusInternalServerError)
			return
		}
		log.Infoln("Public Key successfully parsed.")
		hashed := sha256.Sum256([]byte(toCompare))
		log.Infoln("Verifying Signature...")
		if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signature); err != nil {
			log.Errorf("failed to verify the provided signature, got err=%v", err)
			http.Error(w, "failed to validate signature", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func processSignatureHeader(header http.Header) (map[string]string, error) {
	httpSignature := header.Get("Signature")
	signature := map[string]string{}
	signaturePairs := strings.Split(httpSignature, ",")
	if len(signaturePairs) < 3 {
		return nil, fmt.Errorf("failed to process signature: unexpected input format, got %d pairs", len(signaturePairs))
	}
	for _, rawPair := range signaturePairs {
		splitPair := strings.SplitN(rawPair, "=", 2)
		signature[splitPair[0]] = strings.Replace(splitPair[1], `"`, "", -1)
	}
	return signature, nil
}
