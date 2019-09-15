package plugins

import (
	"fmt"
	"github.com/lengzhao/libp2p"
	"sync"
)

// SingleConn limit one connection pre user
type SingleConn struct {
	*libp2p.Plugin
	mu    sync.Mutex
	conns map[string]int
}

// Startup Startup
func (p *SingleConn) Startup(net libp2p.Network) {
	p.conns = make(map[string]int)
}

// PeerConnect PeerConnect
func (p *SingleConn) PeerConnect(s libp2p.Session) {
	peer := s.GetPeerAddr()
	id := fmt.Sprintf("%s:%s", peer.Scheme(), peer.User())
	p.mu.Lock()
	n := p.conns[id]
	p.conns[id] = n + 1
	p.mu.Unlock()
	if n > 0 {
		s.Close()
	}
}

// PeerDisconnect PeerDisconnect
func (p *SingleConn) PeerDisconnect(s libp2p.Session) {
	peer := s.GetPeerAddr()
	id := fmt.Sprintf("%s:%s", peer.Scheme(), peer.User())
	p.mu.Lock()
	defer p.mu.Unlock()
	n := p.conns[id]
	if n > 1 {
		p.conns[id] = n - 1
	} else {
		delete(p.conns, id)
	}
}
