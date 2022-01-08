package signature

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
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
		if signatureHeader["keyId"] == "" {
			log.Errorf("keyId not provided")
			http.Error(w, "keyId not provided", http.StatusBadRequest)
			return
		}
		res, err := http.Get(signatureHeader["keyId"])
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
		log.Infoln(marshalledActor)
		if err := json.Unmarshal(marshalledActor, &person); err != nil {
			log.Errorf("failed to unmarshal person, got err=%v", err)
			http.Error(w, "failed to retrieve public key", http.StatusInternalServerError)
			return
		}
		pairs := []string{}
		for _, header := range strings.Split(signatureHeader["headers"], " ") {
			var pair string
			switch header {
			// (request-target) is a fake header that must be constructed using
			// HTTP method and the path.
			case "(request-target)":
				pair = fmt.Sprintf("%s: %s %s", header, strings.ToLower(r.Method), r.URL.Path)
			// Host header is removed from incoming requests and promoted to a
			// field (See: https://pkg.go.dev/net/http#Request).
			case "Host":
				pair = fmt.Sprintf("%s: %s", header, r.Host)
			default:
				pair = fmt.Sprintf("%s: %s", header, r.Header.Get(header))
			}
			pairs = append(pairs, pair)
		}
		toCompare := strings.Join(pairs, "\n")
		block, _ := pem.Decode([]byte(person.PublicKey.PublicKeyPem))
		if block == nil {
			log.Errorf("failed to decode public key from pem")
			http.Error(w, "failed to validate signature", http.StatusInternalServerError)
			return
		}
		publicKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			log.Errorf("failed to parse public key from block")
			http.Error(w, "failed to validate signature", http.StatusInternalServerError)
			return
		}
		hashed := sha256.Sum256([]byte(toCompare))
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
	if len(signaturePairs) != 3 {
		return nil, fmt.Errorf("failed to process signature: unexpected input format, got %d pairs, want 3 pairs", len(signaturePairs))
	}
	for _, rawPair := range signaturePairs {
		splitPair := strings.SplitN(rawPair, "=", 2)
		signature[splitPair[0]] = strings.Replace(splitPair[1], `"`, "", -1)
	}
	return signature, nil
}
