package signature

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
)

// parsePublicKeyFromPEMBlock determines the RSA Public Key from the PEM block.
func parsePublicKeyFromPEMBlock(block []byte) (*rsa.PublicKey, error) {
	pub, err := x509.ParsePKIXPublicKey(block)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key from block: got err=%v", err)
	}
	switch pub.(type) {
	case *rsa.PublicKey:
		return pub.(*rsa.PublicKey), nil
	}
	return nil, fmt.Errorf("unknown public key format parsed")
}
