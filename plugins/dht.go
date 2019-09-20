package plugins

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"expvar"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/lengzhao/libp2p"
	"github.com/lengzhao/libp2p/dht"
)

// Ping dht ping
type Ping struct {
	IsServer bool
}

// Pong dht pong
type Pong struct {
	FromAddr string
	IsServer bool
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
	mu         sync.Mutex
	self       []byte
	discDht    *dht.TDHT // for dht discovery
	conns      map[string]libp2p.Session
	net        libp2p.Network
	address    string
	scheme     string
	cmu        sync.Mutex
	connecting map[string]int64
	findTime   int64
}

const (
	envDHT        = "inDHT"
	envValue      = "true"
	envProtTime   = "protectTime"
	envServerAddr = "address"
)

var stat = expvar.NewMap("dht")

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
	node.PublicKey, _ = hex.DecodeString(un)

	d.self = node.PublicKey
	d.address = node.Address
	d.discDht = dht.CreateDHT(node)
	d.conns = make(map[string]libp2p.Session)
	d.net = net
	d.scheme = u.Scheme
	d.connecting = make(map[string]int64)
}

// Receive is called every time when messages are received
func (d *DiscoveryPlugin) Receive(e libp2p.Event) error {
	// d.mu.Lock()
	// defer d.mu.Unlock()
	stat.Add("event", 1)
	switch msg := e.GetMessage().(type) {
	case Ping:
		// log.Printf("Ping from <%x>\n", e.GetPeerID())
		stat.Add("Ping", 1)
		peer := e.GetSession().GetPeerAddr()
		if msg.IsServer {
			peer.SetServer()
			e.GetSession().SetEnv(envServerAddr, peer.String())
		}
		selfAddr := e.GetSession().GetSelfAddr()
		if selfAddr.IsServer() {
			e.Reply(Pong{selfAddr.String(), true})
		} else {
			peer.SetServer()
			rst := d.addNode(peer.String())
			if rst {
				e.GetSession().SetEnv(envDHT, envValue)
			}
			e.Reply(Pong{d.address, false})
		}
	case Pong:
		// log.Printf("Pong from <%s> %t, self:%s\n", msg.FromAddr, msg.IsServer, e.GetSession().GetSelfAddr())
		stat.Add("Pong", 1)
		e.Reply(Find{d.self})
		peer := e.GetSession().GetPeerAddr()
		if peer.IsServer() || msg.IsServer {
			peer.SetServer()
			e.GetSession().SetEnv(envServerAddr, peer.String())
			rst := d.addNode(peer.String())
			if rst {
				e.GetSession().SetEnv(envDHT, envValue)
			}
		} else {
			u1, _ := url.Parse(peer.String())
			u2, _ := url.Parse(msg.FromAddr)
			if u1 == nil || u2 == nil {
				// log.Println("Pong error addr:", peer.String(), msg.FromAddr)
				return nil
			}
			if u1.User.String() != u2.User.String() {
				return nil
			}
			if u2.Hostname() == "" {
				return nil
			}
			rst := d.addNode(msg.FromAddr)
			if rst {
				e.GetSession().SetEnv(envDHT, envValue)
			}
			if len(msg.FromAddr) < 100 {
				e.GetSession().SetEnv(envServerAddr, msg.FromAddr)
			}
		}
	case Find:
		stat.Add("Find", 1)
		// log.Printf("Find from <%s>\n", e.GetSession().GetPeerAddr())
		peer := new(dht.NodeID)
		peer.PublicKey = msg.Key
		peer.Address = ""
		nodes := d.discDht.Find(peer)
		if len(nodes) == 0 {
			// log.Println("not any node")
			return nil
		}
		resp := new(Nodes)
		resp.Addresses = make([]string, len(nodes))
		for i, node := range nodes {
			resp.Addresses[i] = node.GetAddresses()
		}
		// log.Printf("Find from <%x> %d, self:%s\n", e.GetPeerID(), len(nodes), e.GetSession().GetSelfAddr())
		e.Reply(resp)
	case Nodes:
		stat.Add("Nodes", 1)
		d.findTime = time.Now().Unix()
		for i, addr := range msg.Addresses {
			if i > dht.BucketSize {
				break
			}
			pu, err := url.Parse(addr)
			if err != nil {
				// log.Println("fail to parse adddress:", addr, err)
				continue
			}
			d.mu.Lock()
			if d.conns[pu.User.Username()] != nil {
				d.mu.Unlock()
				continue
			}
			d.mu.Unlock()

			session := d.newConn(addr)
			if session == nil {
				// log.Println("fail to new session:", addr, err)
				continue
			}
			err = session.Send(Ping{IsServer: session.GetSelfAddr().IsServer()})
			if err != nil {
				// log.Println("fail to send ping:", addr, err)
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
		stat.Add("NatTraversal", 1)
		// log.Printf("Traversal peer:<%x>, from:%s, to:%s \n", e.GetPeerID(), msg.FromAddr, msg.ToAddr)
		fu, err := url.Parse(msg.FromAddr)
		if err != nil {
			// log.Println("error address:", msg.FromAddr, err)
			return nil
		}
		tu, err := url.Parse(msg.ToAddr)
		if err != nil {
			// log.Println("error address:", msg.FromAddr, err)
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
			d.mu.Lock()
			if d.conns[fu.User.Username()] != nil {
				d.mu.Unlock()
				return nil
			}
			d.mu.Unlock()
			session := d.newConn(msg.FromAddr)
			if session == nil {
				return nil
			}
			session.Send(Ping{IsServer: session.GetSelfAddr().IsServer()})
			// log.Println("Traversal dst, send DhtPing to:", msg.FromAddr)
		} else if bytes.Compare(fid, e.GetPeerID()) == 0 { //proxy
			d.mu.Lock()
			if d.conns[fu.User.Username()] == nil {
				d.mu.Unlock()
				return nil
			}
			session := d.conns[tu.User.Username()]
			d.mu.Unlock()
			if session == nil {
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
		session := e.GetSession()
		if session.GetEnv(envDHT) == envValue {
			// log.Println("session in dht")
			return nil
		}
		t := fmt.Sprintf("%d", time.Now().Unix())
		pt := session.GetEnv(envProtTime)
		if t <= pt {
			return nil
		}
		d.mu.Lock()
		if len(d.conns) < 20 {
			d.mu.Unlock()
			return nil
		}
		d.mu.Unlock()
		session.Close()
	}
	return nil
}

// if is new node,return true
func (d *DiscoveryPlugin) addNode(address string) (bNew bool) {
	bNew = false
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
	bNew = d.discDht.Add(node)
	// if bNew {
	// 	log.Println("dht add address:", address)
	// }
	return
}

// PeerConnect is called every time a PeerSession is initialized and connected
func (d *DiscoveryPlugin) PeerConnect(s libp2p.Session) {
	t := fmt.Sprintf("%d", time.Now().Add(5*time.Second).Unix())
	s.SetEnv(envProtTime, t)
	un := s.GetPeerAddr().User()
	go s.Send(Ping{IsServer: s.GetSelfAddr().IsServer()})
	d.cmu.Lock()
	delete(d.connecting, un)
	d.cmu.Unlock()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.conns[un] = s
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
	d.discDht.RemoveNode(&node)
	delete(d.conns, un)
}

func (d *DiscoveryPlugin) newConn(addr string) libp2p.Session {
	u, err := url.Parse(addr)
	if err != nil || u.User == nil {
		return nil
	}
	user := u.User.Username()
	if user == "" {
		return nil
	}
	var conn libp2p.Session
	d.mu.Lock()
	conn = d.conns[user]
	d.mu.Unlock()
	if conn != nil {
		return conn
	}
	now := time.Now().Unix()
	d.cmu.Lock()
	for k, v := range d.connecting {
		if v < now {
			delete(d.connecting, k)
		}
	}
	t := d.connecting[user]
	if t > now || len(d.connecting) > 100 {
		d.cmu.Unlock()
		// log.Println("try to reconnect:", user)
		return nil
	}
	d.connecting[user] = now + int64(time.Second*10)
	d.cmu.Unlock()
	conn, _ = d.net.NewSession(addr)
	return conn
}
