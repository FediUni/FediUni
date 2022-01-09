package signature

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
)

// parsePublicKeyFromPEMBlock determines the RSA Public Key from PEM.
func parsePublicKeyFromPEMBlock(publicKeyPEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode public key from pem")
	}
	if strings.HasPrefix(publicKeyPEM, "-----BEGIN RSA PUBLIC KEY-----") {
		publicKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse public key from block: got err=%v", err)

		}
		return publicKey, nil
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key from block: got err=%v", err)
	}
	switch pub.(type) {
	case *rsa.PublicKey:
		return pub.(*rsa.PublicKey), nil
	}
	return nil, fmt.Errorf("unknown public key format parsed")
}
