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

// NewMgr new manager
func NewMgr() *Manager {
	out := new(Manager)
	out.pool = make(map[string]libp2p.SignKey)
	out.privKey = make([]byte, 32)
	rand.Read(out.privKey)
	return out
}

// GetDefaultMgr create default manager,use NilKey
func GetDefaultMgr() *Manager {
	out := NewMgr()
	k := new(NilKey)
	out.Register(k)
	out.privKey = make([]byte, 32)
	rand.Read(out.privKey)
	out.SetPrivKey(k.GetType(), nil)
	return out
}

// SetPrivKey get
func (m *Manager) SetPrivKey(typ string, key []byte) error {
	_, ok := m.pool[typ]
	if !ok {
		return errors.New("not exist")
	}
	m.signType = typ
	if len(key) > 0 {
		m.privKey = key
	}
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
