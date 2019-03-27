package network

import (
	"github.com/lengzhao/libp2p"
)

// Event event
type Event struct {
	session  libp2p.Session
	ID       int
	SignType string
	From     []byte
	To       []byte
	Time     int64
	Info     interface{}
}

// Reply reply message
func (c *Event) Reply(msg interface{}) error {
	return c.session.Send(msg)

}

// GetMessage get message
func (c *Event) GetMessage() interface{} {
	return c.Info
}

// GetSession get session
func (c *Event) GetSession() libp2p.Session {
	return c.session
}

// GetPeerID get peer id
func (c *Event) GetPeerID() []byte {
	return c.From
}
