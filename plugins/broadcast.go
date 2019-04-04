package plugins

import (
	"github.com/lengzhao/libp2p"
	"sync"
)

// Broadcast broadcast message to all connection
type Broadcast struct {
	*libp2p.Plugin
	mu    sync.Mutex
	conns map[string]libp2p.Session
	limit int
}

// NewBroadcast new
func NewBroadcast(limit int) *Broadcast {
	out := new(Broadcast)
	out.conns = make(map[string]libp2p.Session)
	out.limit = 3000
	if limit > 0 {
		out.limit = limit
	}
	return out
}

// Broadcast Broadcast message
func (b *Broadcast) Broadcast(msg interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, conn := range b.conns {
		conn.Send(msg)
	}
}

// RandSend send message to one connection
func (b *Broadcast) RandSend(msg interface{}) error {
	var conn libp2p.Session
	b.mu.Lock()
	for _, c := range b.conns {
		conn = c
		break
	}
	b.mu.Unlock()
	return conn.Send(msg)
}

// PeerConnect is called every time a Session is initialized and connected
func (b *Broadcast) PeerConnect(s libp2p.Session) {
	if b.limit <= len(b.conns) {
		return
	}
	key := s.GetPeerAddr().Host()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.conns[key] = s
}

// PeerDisconnect is called every time a Session connection is closed
func (b *Broadcast) PeerDisconnect(s libp2p.Session) {
	key := s.GetPeerAddr().Host()
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.conns, key)
}

// RecInternalMsg internal msg
func (b *Broadcast) RecInternalMsg(msg libp2p.InterMsg) error {
	switch msg.GetType() {
	case "broadcast":
		b.Broadcast(msg.GetMsg())
	case "randsend":
		b.RandSend(msg.GetMsg())
	}
	return nil
}
