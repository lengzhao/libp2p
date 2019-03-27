package network

import (
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/lengzhao/libp2p"
	"github.com/lengzhao/libp2p/conn"
	"github.com/lengzhao/libp2p/crypto"
	"log"
	"net/url"
	"sync"
)

// Manager network manager
type Manager struct {
	mu       sync.Mutex
	address  string
	scheme   string
	plugins  []libp2p.IPlugin
	active   bool
	connPool libp2p.ConnPoolMgr
	cryp     libp2p.CryptoMgr
}

// New new network manager
func New() *Manager {
	out := new(Manager)
	out.plugins = make([]libp2p.IPlugin, 0)
	out.connPool = conn.DefaultMgr
	out.cryp = crypto.DefaultMgr
	return out
}

// SetConnPoolMgr set manager of connection pool
func (m *Manager) SetConnPoolMgr(p libp2p.ConnPoolMgr) {
	m.connPool = p
}

// SetKey set key of sign
func (m *Manager) SetKey(key libp2p.CryptoMgr) {
	m.cryp = key
}

// GetAddress get address
func (m *Manager) GetAddress() string {
	return m.address
}

// Listen listen
func (m *Manager) Listen(address string) error {
	if m.active {
		return errors.New("error status,it is active")
	}
	u, err := url.Parse(address)
	if err != nil {
		return err
	}
	m.scheme = u.Scheme
	id := hex.EncodeToString(m.cryp.GetPublic())
	u.User = url.User(id)
	m.address = u.String()
	m.active = true

	for _, plugin := range m.plugins {
		plugin.Startup(m)
		defer plugin.Cleanup(m)
	}

	log.Println("listen address:", m.address)
	return m.connPool.Listen(m.address, m.process)
}

// NewSession new connection
func (m *Manager) NewSession(address string) (libp2p.Session, error) {
	if !m.active {
		return nil, errors.New("error status,it is not active")
	}

	u, err := url.Parse(address)
	if err != nil {
		log.Println("fail to parse address.addr:", address, err)
		return nil, err
	}
	if u.User == nil {
		return nil, errors.New("error user of the address")
	}
	id, err := hex.DecodeString(u.User.Username())
	if err != nil {
		return nil, errors.New("error user of the address")
	}
	if bytes.Compare(id, m.cryp.GetPublic()) == 0 {
		return nil, errors.New("try to connect self")
	}

	conn, err := m.connPool.Dial(address)
	if err != nil {
		return nil, err
	}
	s := newSession(m, conn, id, false)
	return s, nil
}

func (m *Manager) process(conn libp2p.Conn) {
	newSession(m, conn, nil, true)
}

// RegistPlugin regist plugin
func (m *Manager) RegistPlugin(p libp2p.IPlugin) {
	if m.active {
		panic("not support,manager is actived")
	}
	m.mu.Lock()
	m.plugins = append(m.plugins, p)
	m.mu.Unlock()
}

// SendInternalMsg send internal message
func (m *Manager) SendInternalMsg(msg libp2p.InterMsg) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.plugins {
		p.RecInternalMsg(msg)
	}
}

// Close close manager
func (m *Manager) Close() {
	m.connPool.Close()
}
