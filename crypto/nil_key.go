package crypto

import (
	"bytes"
)

// NilPrivKey nil priv key, just one example
type NilPrivKey struct {
	key []byte
}

// NilKey nil key, just one example
type NilKey struct {
}

// GetType get type
func (k *NilKey) GetType() string {
	return "nil"
}

// GetPrivKey get private key
func (k *NilKey) GetPrivKey(key []byte) IPrivKey {
	return &NilPrivKey{key}
}

// Verify verify
func (k *NilKey) Verify(data, sig, pubKey []byte) bool {
	if bytes.Compare(sig, pubKey) == 0 {
		return true
	}
	return false
}

// Bytes private key output
func (pk *NilPrivKey) Bytes() []byte {
	return pk.key
}

// Sign sign data
func (pk *NilPrivKey) Sign(data []byte) []byte {
	return pk.key
}

// GetPublic get public key
func (pk *NilPrivKey) GetPublic() []byte {
	return pk.key
}
