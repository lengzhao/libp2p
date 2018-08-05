package dht

import (
	"bytes"
	"encoding/hex"
	"github.com/lengzhao/libp2p"
	"github.com/lengzhao/libp2p/message"
	"log"
	"net/url"
	"sync"
	"time"
)

type tReconnectNode struct {
	timeout int64
	address string
}

// DiscoveryPlugin discovery plugin of dht
type DiscoveryPlugin struct {
	*libp2p.Plugin
	mu     sync.Mutex
	self   []byte
	dht    *TDHT
	rcList []*tReconnectNode
}

// Startup is called only once when the plugin is loaded
func (d *DiscoveryPlugin) Startup(net *libp2p.Network) {
	node := new(NodeID)
	node.PublicKey = net.GetID()
	node.Address = net.GetAddress()
	d.self = node.PublicKey
	d.dht = CreateDHT(node)
	d.rcList = make([]*tReconnectNode, 0)
}

func (d *DiscoveryPlugin) doReConn(n *libp2p.Network) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.rcList) == 0 {
		return
	}
	now := time.Now().Unix()
	addrs := make([]string, 0)
	for _, node := range d.rcList {
		if node.timeout > now {
			break
		}
		addrs = append(addrs, node.address)
	}
	if len(addrs) == 0 {
		return
	}
	d.rcList = d.rcList[len(addrs):]
	go n.Bootstrap(addrs...)
}

// Receive is called every time when messages are received
func (d *DiscoveryPlugin) Receive(ctx *libp2p.PluginContext) error {
	d.doReConn(ctx.Session.Net)
	d.mu.Lock()
	defer d.mu.Unlock()
	switch msg := ctx.GetMessage().(type) {
	case *message.DhtPing:
		log.Printf("Ping from <%s>\n", ctx.Session.GetRemoteAddress())
		ctx.Reply(new(message.DhtPong))
		rst := d.addNode(ctx.Session.GetRemoteAddress())
		if rst {
			ctx.Reply(new(message.DhtFind))
		}
	case *message.DhtPong:
		log.Printf("Pong from <%s>\n", ctx.Session.GetRemoteAddress())
		ctx.Reply(new(message.DhtFind))
		d.addNode(ctx.Session.GetRemoteAddress())
	case *message.DhtFind:
		log.Printf("Find from <%s>\n", ctx.Session.GetRemoteAddress())
		d.addNode(ctx.Session.GetRemoteAddress())
		peer := new(NodeID)
		peer.PublicKey = ctx.Session.GetPeerID()
		peer.Address = ctx.Session.GetRemoteAddress()
		nodes := d.dht.Find(peer)
		if len(nodes) == 0 {
			log.Println("not any node")
			return nil
		}
		resp := new(message.DhtNodes)
		resp.Addresses = make([]string, len(nodes))
		for i, node := range nodes {
			resp.Addresses[i] = node.GetAddresses()
		}
		ctx.Reply(resp)
	case *message.DhtNodes:
		for _, addr := range msg.Addresses {
			rst := d.addNode(addr)
			if !rst {
				continue
			}
			session := ctx.Session.Net.NewSession(addr)
			if session == nil {
				continue
			}
			session.Send(new(message.DhtPing))
			trav := new(message.NatTraversal)
			trav.FromAddr = ctx.Session.Net.GetAddress()
			trav.ToAddr = addr
			ctx.Reply(trav)
		}
	case *message.NatTraversal:
		log.Printf("Traversal peer:<%x>, from:%s, to:%s \n", ctx.GetRemoteID(), msg.FromAddr, msg.ToAddr)
		fu, err := url.Parse(msg.FromAddr)
		if err != nil {
			log.Println("error address:", msg.FromAddr, err)
			return nil
		}
		tu, err := url.Parse(msg.ToAddr)
		if err != nil {
			log.Println("error address:", msg.FromAddr, err)
			return nil
		}
		fid, err := hex.DecodeString(fu.User.Username())
		if err != nil {
			return nil
		}
		tid, err := hex.DecodeString(tu.User.Username())
		if err != nil {
			return nil
		}
		// dst
		if bytes.Compare(tid, d.self) == 0 {
			if bytes.Compare(fid, d.self) == 0 {
				return nil
			}
			session := ctx.Session.Net.NewSession(msg.FromAddr)
			if session == nil {
				return nil
			}
			session.Send(new(message.DhtPing))
			log.Println("Traversal dst, send DhtPing to:", msg.FromAddr)
		} else if bytes.Compare(fid, ctx.GetRemoteID()) == 0 { //proxy
			if bytes.Compare(tid, d.self) == 0 {
				return nil
			}
			session := ctx.Session.Net.NewSession(msg.ToAddr)
			if session == nil {
				return nil
			}
			msg.FromAddr = ctx.Session.GetRemoteAddress()
			session.Send(msg)
		}
	}
	return nil
}

// if is new node,return true
func (d *DiscoveryPlugin) addNode(address string) (bNew bool) {
	bNew = false
	//log.Println("dht try add address:", address)
	u, err := url.Parse(address)
	if err != nil {
		return
	}
	id, err := hex.DecodeString(u.User.Username())
	if err != nil {
		return
	}
	if bytes.Compare(d.self, id) == 0 {
		return
	}
	node := new(NodeID)
	node.PublicKey = id
	node.Address = address
	bNew = d.dht.Add(node)
	if bNew {
		log.Println("dht add address:", address)
	}
	return
}

// PeerDisconnect is called every time a PeerSession connection is closed
func (d *DiscoveryPlugin) PeerDisconnect(s *libp2p.PeerSession) {
	node := NodeID{}
	node.PublicKey = s.GetPeerID()
	if len(node.PublicKey) == 0 {
		return
	}
	node.Address = s.GetRemoteAddress()
	rcNode := tReconnectNode{}
	rcNode.address = node.Address
	rcNode.timeout = time.Now().Add(time.Minute * 5).Unix()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dht.RemoveNode(&node)
	if len(d.rcList) > 1000 {
		return
	}
	d.rcList = append(d.rcList, &rcNode)
}
