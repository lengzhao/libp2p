package crypto

// IPrivKey represents a private key that can be used to generate a public key,
// sign data, and decrypt data that was encrypted with a public key
type IPrivKey interface {
	Bytes() []byte
	// Cryptographically sign the given bytes
	Sign(data []byte) []byte
	// Return a public key paired with this private key
	GetPublic() []byte
}

// IKey interface define of crypto key
type IKey interface {
	GetType() string
	GetPrivKey(key []byte) IPrivKey
	Verify(data, sig, pubKey []byte) bool
}
