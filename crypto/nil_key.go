package crypto

import (
	"bytes"
)

// NilKey nil key, just one example
type NilKey struct {
}

// GetType get type
func (k *NilKey) GetType() string {
	return "nil"
}

// Verify verify
func (k *NilKey) Verify(data, sig, pubKey []byte) bool {
	if bytes.Compare(sig, pubKey) == 0 {
		return true
	}
	return false
}

// Sign sign data
func (k *NilKey) Sign(data, privKey []byte) []byte {
	return privKey[:6]
}

// GetPublic get public key
func (k *NilKey) GetPublic(privKey []byte) []byte {
	return privKey[:6]
}
