package plugins

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"github.com/lengzhao/libp2p"
	"github.com/lengzhao/libp2p/dht"
	"log"
	"net/url"
	"sync"
)

// Ping dht ping
type Ping struct {
	FromAddr string
}

// Pong dht pong
type Pong struct {
	FromAddr string
}

// Find find other nodes
type Find struct {
	Key []byte
}

// Nodes nodes
type Nodes struct {
	Addresses []string
}

// NatTraversal try nat traversal by proxy node
type NatTraversal struct {
	FromAddr string
	ToAddr   string
}

// DiscoveryPlugin discovery plugin of dht
type DiscoveryPlugin struct {
	*libp2p.Plugin
	mu      sync.Mutex
	self    []byte
	dht     *dht.TDHT
	conns   map[string]bool
	Filter  bool
	address string
	net     libp2p.Network
	scheme  string
}

func init() {
	gob.Register(Ping{})
	gob.Register(Pong{})
	gob.Register(Find{})
	gob.Register(Nodes{})
	gob.Register(NatTraversal{})
}

// Startup is called only once when the plugin is loaded
func (d *DiscoveryPlugin) Startup(net libp2p.Network) {
	node := new(dht.NodeID)
	node.Address = net.GetAddress()
	u, _ := url.Parse(node.Address)
	un := u.User.Username()
	if len(un) <= 32 {
		node.PublicKey = []byte(un)
	} else {
		node.PublicKey, _ = hex.DecodeString(un)
	}

	d.self = node.PublicKey
	d.dht = dht.CreateDHT(node)
	d.conns = make(map[string]bool)
	d.address = node.Address
	d.net = net
	d.scheme = u.Scheme
}

// Receive is called every time when messages are received
func (d *DiscoveryPlugin) Receive(e libp2p.Event) error {
	// d.mu.Lock()
	// defer d.mu.Unlock()
	switch msg := e.GetMessage().(type) {
	case Ping:
		// log.Printf("Ping from <%s>\n", msg.FromAddr)
		e.Reply(Pong{d.address})
		peer := e.GetSession().GetPeerAddr()
		if peer.IsServer() {
			rst := d.addNode(peer.String())
			if rst {
				e.Reply(Find{d.self})
			}
		} else {
			u1, _ := url.Parse(peer.String())
			u2, _ := url.Parse(msg.FromAddr)
			if u1 == nil || u2 == nil {
				return nil
			}
			if u1.Hostname() != u2.Hostname() {
				return nil
			}
			rst := d.addNode(msg.FromAddr)
			if rst {
				e.Reply(Find{d.self})
			}
		}

	case Pong:
		// log.Printf("Pong from <%s>\n", msg.FromAddr)
		e.Reply(Find{d.self})
		peer := e.GetSession().GetPeerAddr()
		if peer.IsServer() {
			d.addNode(peer.String())
		} else {
			u1, _ := url.Parse(peer.String())
			u2, _ := url.Parse(msg.FromAddr)
			if u1 == nil || u2 == nil {
				return nil
			}
			if u1.Hostname() != u2.Hostname() {
				return nil
			}
			d.addNode(msg.FromAddr)
		}
	case Find:
		// log.Printf("Find from <%s>\n", e.GetSession().GetPeerAddr())
		peer := new(dht.NodeID)
		peer.PublicKey = msg.Key
		peer.Address = ""
		nodes := d.dht.Find(peer)
		if len(nodes) == 0 {
			log.Println("not any node")
			return nil
		}
		resp := new(Nodes)
		resp.Addresses = make([]string, len(nodes))
		for i, node := range nodes {
			resp.Addresses[i] = node.GetAddresses()
		}
		e.Reply(resp)
	case Nodes:
		for _, addr := range msg.Addresses {
			pu, err := url.Parse(addr)
			if err != nil {
				log.Println("fail to parse adddress:", addr, err)
				continue
			}

			if d.conns[pu.User.Username()] {
				continue
			}

			session, err := d.net.NewSession(addr)
			if err != nil {
				log.Println("fail to new session:", addr, err)
				continue
			}
			err = session.Send(Ping{d.address})
			if err != nil {
				log.Println("fail to send ping:", addr, err)
				continue
			}
			if pu.Scheme == d.scheme &&
				e.GetSession().GetPeerAddr().IsServer() &&
				e.GetSession().GetSelfAddr().IsServer() {
				trav := new(NatTraversal)
				trav.FromAddr = d.address
				trav.ToAddr = addr
				e.Reply(trav)
			}
		}
	case NatTraversal:
		// log.Printf("Traversal peer:<%x>, from:%s, to:%s \n", e.GetPeerID(), msg.FromAddr, msg.ToAddr)
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
			session, err := d.net.NewSession(msg.FromAddr)
			if err != nil {
				return nil
			}
			session.Send(Ping{d.address})
			// log.Println("Traversal dst, send DhtPing to:", msg.FromAddr)
		} else if bytes.Compare(fid, e.GetPeerID()) == 0 { //proxy
			if !d.conns[tu.User.Username()] {
				return nil
			}
			if !d.conns[fu.User.Username()] {
				return nil
			}
			session, err := d.net.NewSession(msg.ToAddr)
			if err != nil {
				return nil
			}
			peer := e.GetSession().GetPeerAddr().String()
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
		address := e.GetSession().GetPeerAddr().String()
		u, err := url.Parse(address)
		if err != nil {
			panic(err)
		}
		id, err := hex.DecodeString(u.User.Username())
		if err != nil {
			panic(err)
		}
		node := new(dht.NodeID)
		node.PublicKey = id
		node.Address = address
		exist := d.dht.NodeExists(node)
		if !exist {
			e.GetSession().Close()
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
	node := new(dht.NodeID)
	node.PublicKey = id
	node.Address = address
	d.dht.Add(node)
	// bNew = d.dht.Add(node)
	// if bNew {
	// 	log.Println("dht add address:", address)
	// }
	return
}

// PeerConnect is called every time a PeerSession is initialized and connected
func (d *DiscoveryPlugin) PeerConnect(s libp2p.Session) {
	un := s.GetPeerAddr().User()
	go s.Send(Ping{d.address})
	// log.Println("new peer:", un, len(d.conns))
	d.mu.Lock()
	defer d.mu.Unlock()
	d.conns[un] = true
}

// PeerDisconnect is called every time a PeerSession connection is closed
func (d *DiscoveryPlugin) PeerDisconnect(s libp2p.Session) {
	node := dht.NodeID{}
	un := s.GetPeerAddr().User()
	if un == "" {
		return
	}
	if len(un) <= 32 {
		node.PublicKey = []byte(un)
	} else {
		node.PublicKey, _ = hex.DecodeString(un)
	}
	if len(node.PublicKey) == 0 {
		return
	}
	node.Address = s.GetPeerAddr().String()
	// log.Println("peer leave:", un, len(d.conns))
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dht.RemoveNode(&node)
	delete(d.conns, un)
}
