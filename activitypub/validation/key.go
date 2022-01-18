package validation

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// parsePublicKeyFromPEMBlock determines the RSA Public Key from PEM.
func parsePublicKeyFromPEMBlock(publicKeyPEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode public key from pem")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		publicKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse public key from block: got err=%v", err)
		}
		return publicKey, nil
	}
	switch pub.(type) {
	case *rsa.PublicKey:
		return pub.(*rsa.PublicKey), nil
	}
	return nil, fmt.Errorf("unknown public key format parsed")
}

func ParsePrivateKeyFromPEMBlock(privateKeyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}
