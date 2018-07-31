package libp2p

import (
	"encoding/hex"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/lengzhao/libp2p/crypto"
	"log"
)

// PluginContext provides parameters and helper functions to a Plugin
// for interacting with/analyzing incoming messages from a select peer.
type PluginContext struct {
	Session *PeerSession
	message proto.Message
}

func newContext(c *PeerSession, data []byte) error {
	var msg crypto.Message
	err := proto.Unmarshal(data, &msg)
	if err != nil {
		return err
	}
	var ptr types.DynamicAny
	err = types.UnmarshalAny(msg.DataMsg, &ptr)
	if err != nil {
		return err
	}

	log.Printf("new msg:%t,%v\n", ptr.Message, ptr.Message)
	ctx := new(PluginContext)
	ctx.Session = c
	ctx.message = ptr.Message
	for _, plugin := range c.Net.pulgins {
		err = plugin.Receive(ctx)
		if err != nil {
			log.Println("fail to do plugin.Receive.", plugin)
		}
	}
	return err
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
func (ctx *PluginContext) GetRemoteID() string {
	if ctx.Session.peerID == nil {
		return ""
	}
	return hex.EncodeToString(ctx.Session.peerID)
}
