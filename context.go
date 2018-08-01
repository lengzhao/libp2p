package libp2p

import (
	"github.com/gogo/protobuf/proto"
	"log"
)

// PluginContext provides parameters and helper functions to a Plugin
// for interacting with/analyzing incoming messages from a select peer.
type PluginContext struct {
	Session *PeerSession
	message proto.Message
}

func newContext(c *PeerSession, message proto.Message) error {
	ctx := new(PluginContext)
	ctx.Session = c
	ctx.message = message
	for _, plugin := range c.Net.pulgins {
		err := plugin.Receive(ctx)
		if err != nil {
			log.Println("fail to do plugin.Receive.", plugin)
		}
	}
	return nil
}

// GetMessage get message
func (ctx *PluginContext) GetMessage() proto.Message {
	return ctx.message
}

// Reply reply message
func (ctx *PluginContext) Reply(msg proto.Message) error {
	return ctx.Session.Send(msg)
}

// GetRemoteID get remote id
func (ctx *PluginContext) GetRemoteID() []byte {
	return ctx.Session.peerID
}
