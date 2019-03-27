package crypto

import (
	"crypto/rand"
	"errors"
	"github.com/lengzhao/libp2p"
)

// Manager manager
type Manager struct {
	pool     map[string]libp2p.SignKey
	privKey  []byte
	signType string
}

// DefaultMgr default manager
var DefaultMgr *Manager

func init() {
	DefaultMgr = new(Manager)
	DefaultMgr.pool = make(map[string]libp2p.SignKey)
	DefaultMgr.privKey = make([]byte, 32)
	rand.Read(DefaultMgr.privKey)
	k := new(NilKey)
	DefaultMgr.Register(k)
	DefaultMgr.signType = k.GetType()
}

// SetPrivKey get
func (m *Manager) SetPrivKey(typ string, key []byte) error {
	_, ok := m.pool[typ]
	if !ok {
		return errors.New("not exist")
	}
	m.signType = typ
	m.privKey = key
	return nil
}

// GetType get sign type
func (m *Manager) GetType() string {
	return m.signType
}

// Sign sign
func (m *Manager) Sign(data []byte) []byte {
	p := m.pool[m.signType]
	return p.Sign(data, m.privKey)
}

// Verify Verify
func (m *Manager) Verify(typ string, data, sig, pubKey []byte) bool {
	p := m.pool[typ]
	if p == nil {
		return false
	}
	return p.Verify(data, sig, pubKey)
}

// Register Register
func (m *Manager) Register(k libp2p.SignKey) {
	t := k.GetType()
	_, ok := m.pool[t]
	if ok {
		panic("exist signKey")
	}

	m.pool[t] = k
}

// GetPublic GetPublic
func (m *Manager) GetPublic() []byte {
	p := m.pool[m.signType]
	return p.GetPublic(m.privKey)
}
