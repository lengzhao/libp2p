package dht

import (
	"github.com/lengzhao/libp2p"
	"github.com/lengzhao/libp2p/message"
	"log"
)

// DiscoveryPlugin discovery plugin of dht
type DiscoveryPlugin struct {
	*libp2p.Plugin
	dht *TDHT
}

// Startup is called only once when the plugin is loaded
func (d *DiscoveryPlugin) Startup(net *libp2p.Network) {
	node := new(NodeID)
	node.PublicKey = net.GetID()
	node.Address = net.GetAddress()
	d.dht = CreateDHT(node)
}

// Receive is called every time when messages are received
func (d *DiscoveryPlugin) Receive(ctx *libp2p.PluginContext) error {
	switch msg := ctx.GetMessage().(type) {
	case *message.DhtPing:
		log.Printf("ping from <%x>\n", ctx.GetRemoteID())
	case *message.DhtPong:
		log.Printf("Pong from <%x>\n", ctx.GetRemoteID())
	case *message.DhtFind:
		log.Printf("Find from <%x>\n", ctx.GetRemoteID())
	case *message.NatTraversal:
		log.Printf("Traversal peer:<%x>, from:%s, to:%s \n", ctx.GetRemoteID(), msg.FromAddr, msg.ToAddr)
	}
	return nil
}

// PeerDisconnect is called every time a PeerSession connection is closed
func (*DiscoveryPlugin) PeerDisconnect(s *libp2p.PeerSession) {}
