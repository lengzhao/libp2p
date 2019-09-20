package plugins

import (
	"errors"
	"github.com/lengzhao/libp2p"
	"math/rand"
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
	var lst []libp2p.Session
	b.mu.Lock()
	lst = make([]libp2p.Session, len(b.conns))
	var i int
	for _, conn := range b.conns {
		lst[i] = conn
		i++
	}
	b.mu.Unlock()
	for _, conn := range lst {
		conn.Send(msg)
	}
}

// RandSend send message to one connection
func (b *Broadcast) RandSend(msg interface{}) error {
	var conn libp2p.Session
	r := rand.Int()
	if r < 0 {
		r = 0 - r
	}
	b.mu.Lock()
	if len(b.conns) == 0 {
		return errors.New("not exist any connection")
	}
	index := r % len(b.conns)
	for _, c := range b.conns {
		if index > 0 {
			index--
			continue
		}
		conn = c
		break
	}
	b.mu.Unlock()
	if conn == nil {
		return errors.New("not exist any connection")
	}
	return conn.Send(msg)
}

// PeerConnect is called every time a Session is initialized and connected
func (b *Broadcast) PeerConnect(s libp2p.Session) {
	if b.limit <= len(b.conns) {
		return
	}
	key := s.GetPeerAddr().User()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.conns[key] = s
}

// PeerDisconnect is called every time a Session connection is closed
func (b *Broadcast) PeerDisconnect(s libp2p.Session) {
	key := s.GetPeerAddr().User()
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
