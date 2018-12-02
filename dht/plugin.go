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

// DiscoveryPlugin discovery plugin of dht
type DiscoveryPlugin struct {
	*libp2p.Plugin
	mu            sync.Mutex
	self          []byte
	dht           *TDHT
	conns         map[string]bool
	lastDiscovery int64
	Filter        bool
}

// Startup is called only once when the plugin is loaded
func (d *DiscoveryPlugin) Startup(net *libp2p.Network) {
	node := new(NodeID)
	node.PublicKey = net.GetID()
	node.Address = net.GetAddress()
	d.self = node.PublicKey
	d.dht = CreateDHT(node)
	d.conns = make(map[string]bool)
	d.lastDiscovery = time.Now().Unix()
}

// Receive is called every time when messages are received
func (d *DiscoveryPlugin) Receive(ctx *libp2p.PluginContext) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if time.Now().Unix() > d.lastDiscovery+7200 {
		d.lastDiscovery = time.Now().Unix()
		d.mu.Unlock()
		ctx.Session.Net.Broadcast(new(message.DhtFind))
		d.mu.Lock()
	}
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
		d.lastDiscovery = time.Now().Unix()
		for _, addr := range msg.Addresses {
			pu, err := url.Parse(addr)
			if err != nil {
				continue
			}

			if d.conns[pu.User.Username()] {
				continue
			}

			session := ctx.Session.Net.NewSession(addr)
			if session == nil {
				continue
			}
			session.Send(new(message.DhtPing))
		}
		go func() {
			time.Sleep(1 * time.Second)
			for _, addr := range msg.Addresses {
				pu, err := url.Parse(addr)
				if err != nil {
					continue
				}
				id, err := hex.DecodeString(pu.User.Username())
				if bytes.Compare(id, ctx.Session.Net.GetID()) == 0 {
					continue
				}

				d.mu.Lock()
				if d.conns[pu.User.Username()] {
					d.mu.Unlock()
					continue
				}
				d.mu.Unlock()

				trav := new(message.NatTraversal)
				trav.FromAddr = ctx.Session.Net.GetAddress()
				trav.ToAddr = addr
				ctx.Reply(trav)
			}
		}()
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
			if d.conns[fu.User.Username()] {
				return nil
			}
			session := ctx.Session.Net.NewSession(msg.FromAddr)
			if session == nil {
				return nil
			}
			session.Send(new(message.DhtPing))
			log.Println("Traversal dst, send DhtPing to:", msg.FromAddr)
		} else if bytes.Compare(fid, ctx.GetRemoteID()) == 0 { //proxy
			if !d.conns[tu.User.Username()] {
				return nil
			}
			if !d.conns[fu.User.Username()] {
				return nil
			}
			session := ctx.Session.Net.NewSession(msg.ToAddr)
			if session == nil {
				return nil
			}
			peer := ctx.Session.GetRemoteAddress()
			pu, err := url.Parse(peer)
			if err == nil {
				if pu.Hostname() != tu.Hostname() {
					msg.FromAddr = peer
				}
			}
			session.Send(msg)
		}
	default:
		if !d.Filter {
			return nil
		}
		address := ctx.Session.GetRemoteAddress()
		u, err := url.Parse(address)
		if err != nil {
			panic(err)
		}
		id, err := hex.DecodeString(u.User.Username())
		if err != nil {
			panic(err)
		}
		node := new(NodeID)
		node.PublicKey = id
		node.Address = address
		exist := d.dht.NodeExists(node)
		if !exist {
			ctx.Session.Close(true)
			panic("not in dht")
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

// PeerConnect is called every time a PeerSession is initialized and connected
func (d *DiscoveryPlugin) PeerConnect(s *libp2p.PeerSession) {
	pu, err := url.Parse(s.GetRemoteAddress())
	if err != nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.conns[pu.User.Username()] = true
	log.Println("new peer:", pu.User.Username(), len(d.conns))
}

// PeerDisconnect is called every time a PeerSession connection is closed
func (d *DiscoveryPlugin) PeerDisconnect(s *libp2p.PeerSession) {
	node := NodeID{}
	node.PublicKey = s.GetPeerID()
	if len(node.PublicKey) == 0 {
		return
	}
	node.Address = s.GetRemoteAddress()
	pu, _ := url.Parse(node.Address)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dht.RemoveNode(&node)
	delete(d.conns, pu.User.Username())
	log.Println("peer leave:", pu.User.Username(), len(d.conns))
}
