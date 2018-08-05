package libp2p

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"github.com/lengzhao/libp2p/crypto"
	"github.com/lengzhao/libp2p/message"
	"github.com/xtaci/kcp-go"
	"log"
	"math/rand"
	"net"
	"net/url"
	"sync"
	"time"
)

// Network represents the current networking state for this node.
type Network struct {
	*net.UDPConn
	address    string
	addr       *net.UDPAddr
	selfKey    crypto.IPrivKey
	publicKey  []byte
	selfKeygen string
	keygen     map[string]crypto.IKey
	peers      map[string]*PeerSession
	clientRCV  map[string]*clientConn
	pulgins    []IPlugin
	started    bool
	die        chan bool
	mu         sync.Mutex
	listener   *kcp.Listener
	timeWait   []*clientConn
}

// NewNetwork create a new network,listen the addr
// addrStr: kcp://0.0.0.0:3000
func NewNetwork(addrStr string) *Network {
	u, err := url.Parse(addrStr)
	if err != nil {
		log.Println("error address:", addrStr, err)
		return nil
	}
	if u.Scheme != "kcp" {
		log.Println("scheme not support:", u.Scheme)
		return nil
	}

	pn := new(Network)
	pn.address = addrStr
	pn.addr, err = net.ResolveUDPAddr("udp", u.Host)
	if err != nil {
		log.Println("fail to ResolveUDPAddr,", u.Host, err)
		return nil
	}

	pn.pulgins = make([]IPlugin, 0, 10)
	pn.peers = make(map[string]*PeerSession)
	pn.keygen = make(map[string]crypto.IKey)
	pn.clientRCV = make(map[string]*clientConn)
	pn.die = make(chan bool)
	pn.timeWait = make([]*clientConn, 0)
	return pn
}

// AddPlugin add plugin
func (pn *Network) AddPlugin(p ...IPlugin) error {
	if pn.started {
		return fmt.Errorf("network started")
	}
	pn.mu.Lock()
	defer pn.mu.Unlock()
	pn.pulgins = append(pn.pulgins, p...)
	return nil
}

// AddKeygen add keygen
func (pn *Network) AddKeygen(keys ...crypto.IKey) error {
	if pn.started {
		return fmt.Errorf("network started")
	}
	pn.mu.Lock()
	defer pn.mu.Unlock()
	for _, k := range keys {
		t := k.GetType()
		if t == "" {
			log.Println("empty type of keygen")
			continue
		}
		old, ok := pn.keygen[t]
		if ok {
			log.Println("exist the keygen:", t, old, ",discard the newer")
			continue
		}
		pn.keygen[t] = k
	}
	return nil
}

// SetCryptoKey set local sing key
func (pn *Network) SetCryptoKey(keygen string, key []byte) error {
	if pn.started {
		return fmt.Errorf("network started")
	}
	pn.mu.Lock()
	defer pn.mu.Unlock()
	kg, ok := pn.keygen[keygen]
	if !ok {
		return fmt.Errorf("unknow keygen:%s", keygen)
	}
	pn.selfKeygen = keygen
	pn.selfKey = kg.GetPrivKey(key)
	pn.publicKey = pn.selfKey.GetPublic()
	pn.address = fmt.Sprintf("kcp://%x@%s", pn.publicKey, pn.addr.String())
	return nil
}

func (pn *Network) setDefaultKeygen() {
	if len(pn.keygen) == 0 {
		key := new(crypto.NilKey)
		pn.keygen[key.GetType()] = key
	}
	if pn.selfKeygen != "" {
		return
	}
	key := make([]byte, 6)
	rand.Seed(time.Now().UnixNano())
	rand.Read(key)
	for k, v := range pn.keygen {
		pn.selfKeygen = k
		pn.selfKey = v.GetPrivKey(key)
		break
	}
	pn.publicKey = pn.selfKey.GetPublic()
	pn.address = fmt.Sprintf("kcp://%x@%s", pn.publicKey, pn.addr.String())
}

type clientConn struct {
	conn      *net.UDPConn
	rAddr     *net.UDPAddr
	rAddrStr  string
	rdChan    chan []byte
	die       chan bool
	cache     []byte
	rdTimeout time.Duration
	kcpConv   uint32
	rdNum     uint64
}

func newClientConn(conn *net.UDPConn, raddr *net.UDPAddr) *clientConn {
	c := new(clientConn)
	c.conn = conn
	c.rAddr = raddr
	c.rAddrStr = raddr.String()
	c.rdChan = make(chan []byte, 10)
	c.die = make(chan bool)
	c.rdTimeout = time.Hour
	log.Printf("new clientConn.from:%s, to:%s \n", conn.LocalAddr().String(), raddr.String())
	return c
}

// WriteTo redirects all writes to the Write syscall, which is 4 times faster.
func (c *clientConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if c.isClose() {
		return 0, errors.New("closed")
	}
	// log.Printf("write to %s,len:%d\n", c.rAddr.String(), len(b))
	return c.conn.WriteTo(b, c.rAddr)
}

func (c *clientConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	if c.cache != nil {
		return c.readFromCache(b)
	}
	select {
	case c.cache = <-c.rdChan:
		return c.readFromCache(b)
	case <-c.die:
		err = errors.New("closed")
	case <-time.After(c.rdTimeout):
		err = errors.New("timeout")
	}
	return
}
func (c *clientConn) readFromCache(b []byte) (n int, addr net.Addr, err error) {
	if c.cache == nil {
		return
	}
	copy(b, c.cache)
	if len(b) > len(c.cache) {
		n = len(c.cache)
		c.cache = nil
		return
	}
	n = len(b)
	c.cache = c.cache[n:]
	return
}

func (c *clientConn) isClose() bool {
	var out bool
	select {
	case <-c.die:
		out = true
	default:
		out = false
	}
	return out
}

func (c *clientConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *clientConn) Close() error {
	select {
	case <-c.die:
	default:
		close(c.die)
	}
	return nil
}
func (c *clientConn) SetDeadline(t time.Time) error      { return nil }
func (c *clientConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *clientConn) SetWriteDeadline(t time.Time) error { return nil }

// Listen Listen,no block
func (pn *Network) Listen() error {
	var err error
	if pn.started {
		return fmt.Errorf("network started")
	}
	pn.started = true
	defer func() { pn.started = false }()
	pn.setDefaultKeygen()

	pn.UDPConn, err = net.ListenUDP("udp", pn.addr)
	if err != nil {
		log.Println("fail to listen:", pn.addr.String())
		return err
	}
	//pn.listener, err = kcp.ServeConn(nil, 0, 0, pn.UDPConn)
	pn.listener, err = kcp.ServeConn(nil, 0, 0, pn)
	if err != nil {
		return err
	}

	fmt.Println("Listen address:", pn.address)
	for _, plugin := range pn.pulgins {
		plugin.Startup(pn)
		defer plugin.Cleanup(pn)
	}
	for {
		conn, err := pn.listener.AcceptKCP()
		if err != nil {
			break
		}
		go newSession(pn, conn, true)
	}
	return nil
}

// NewSession new session,addr:  kcp://publicKey@0.0.0.0:3000
func (pn *Network) NewSession(addr string) *PeerSession {
	u, err := url.Parse(addr)
	if err != nil {
		log.Println("error address:", addr, err)
		return nil
	}
	if u.Scheme != "kcp" {
		log.Println("unknow scheme,address:", addr)
		return nil
	}
	if u.User.Username() == "" {
		log.Println("unknow publicKey,address:", addr)
		return nil
	}
	id, err := hex.DecodeString(u.User.Username())
	if err != nil {
		log.Println("error publicKey,not hex string:", u.User.Username())
		return nil
	}
	pn.mu.Lock()
	pn.checkTimeWait()
	session, ok := pn.peers[u.Host]
	if ok && session.isClose() {
		rcv, ok := pn.clientRCV[u.Host]
		if ok {
			rcv.rdTimeout = 0
		}
		delete(pn.peers, u.Host)
		delete(pn.clientRCV, u.Host)

	} else if ok {
		if len(session.peerID) == 0 {
			session.peerID = id
		}
		pn.mu.Unlock()
		return session
	}
	pn.mu.Unlock()
	raddr, _ := net.ResolveUDPAddr("udp", u.Host)
	c := newClientConn(pn.UDPConn, raddr)
	conn, err := kcp.NewConn(u.Host, nil, 0, 0, c)
	if err != nil {
		return nil
	}
	c.kcpConv = conn.GetConv()
	pn.mu.Lock()
	pn.clientRCV[raddr.String()] = c
	pn.mu.Unlock()
	session = newSession(pn, conn, false)

	session.peerID = id
	return session
}

func (pn *Network) closeSession(session *PeerSession) {
	pn.mu.Lock()
	defer pn.mu.Unlock()
	rcv, ok := pn.clientRCV[session.remoteAddr]
	if !ok {
		rcv = new(clientConn)
		rcv.die = make(chan bool)
		close(rcv.die)
	}
	rcv.rAddrStr = session.remoteAddr
	rcv.kcpConv = session.conn.GetConv()
	rcv.rdTimeout = time.Duration(time.Now().Add(time.Second * 5).UnixNano())
	pn.timeWait = append(pn.timeWait, rcv)
	// log.Printf("network add timewait:%p, peer:%s\n", pn, session.remoteAddr)
	pn.checkTimeWait()
}

func (pn *Network) checkTimeWait() {
	if len(pn.timeWait) == 0 {
		return
	}
	delNum := 0
	now := time.Duration(time.Now().UnixNano())
	for _, rcv := range pn.timeWait {
		if rcv.rdTimeout > now {
			break
		}
		delNum++
	}
	if len(pn.timeWait) > 10000 && delNum == 0 {
		delNum = 1
	}
	if delNum == 0 {
		return
	}
	for i := 0; i < delNum; i++ {
		rcv := pn.timeWait[i]
		if rcv.rdTimeout == 0 {
			continue
		}
		log.Printf("network delete peer,net:%p, peer:%s\n", pn, rcv.rAddrStr)
		delete(pn.peers, rcv.rAddrStr)
		delete(pn.clientRCV, rcv.rAddrStr)
	}
	pn.timeWait = pn.timeWait[delNum:]
}

// GetID get node id
func (pn *Network) GetID() []byte {
	return pn.publicKey
}

// GetAddress get listen address
func (pn *Network) GetAddress() string {
	return pn.address
}

// Close Close
func (pn *Network) Close() error {
	select {
	case <-pn.die:
		return nil
	default:
		close(pn.die)
		pn.UDPConn.Close()
		pn.started = false
	}
	return nil
}

// ReadFrom udp server ReadFrom
func (pn *Network) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	for {
		n, addr, err = pn.UDPConn.ReadFrom(b)
		if err != nil {
			return
		}
		conv := binary.LittleEndian.Uint32(b)
		// log.Printf("Network receive data.net:%p, from:%s, len:%d,conv:%d\n", pn, addr.String(), n, conv)
		pn.mu.Lock()
		pn.checkTimeWait()
		rcv, ok := pn.clientRCV[addr.String()]
		pn.mu.Unlock()
		if !ok {
			break
		}
		//newwork NAT:new conn,but not reply(droped). then receive peer msg with different kcpConv
		if rcv.rdNum == 0 && rcv.kcpConv != conv {
			pn.mu.Lock()
			s, ok := pn.peers[rcv.rAddrStr]
			pn.mu.Unlock()
			if ok {
				s.Close(false)
				return
			}
		}

		buff := make([]byte, n)
		copy(buff, b)
		select {
		case <-rcv.die:
			//closed,drop the data
			//check queal conv
			conv := binary.LittleEndian.Uint32(buff)
			// log.Println("client closed:", rcv.rAddrStr, conv, n)
			if conv != rcv.kcpConv {
				// log.Println("different conv,remove local session")
				pn.mu.Lock()
				defer pn.mu.Unlock()
				delete(pn.peers, rcv.rAddrStr)
				delete(pn.clientRCV, rcv.rAddrStr)
				rcv.rdTimeout = 0
				return
			}
			continue
		case rcv.rdChan <- buff:
			// log.Printf("client receive data.from:%s, len:%d \n", addr.String(), n)
			if rcv.kcpConv != conv {
				log.Printf("client receive error data.from:%s, len:%d,selfConv:%d,conv:%d \n", addr.String(), n, rcv.kcpConv, conv)
			}
			rcv.rdNum++
		}
	}

	return
}

// Bootstrap connect peers,send message.DhtPing
func (pn *Network) Bootstrap(addresses ...string) {
	for _, addr := range addresses {
		session := pn.NewSession(addr)
		session.Send(&message.DhtPing{})
	}
}

// Broadcast send message to all peer clients.
func (pn *Network) Broadcast(message proto.Message) {
	pn.mu.Lock()
	peers := make([]*PeerSession, 0, len(pn.peers))
	for _, peer := range pn.peers {
		peers = append(peers, peer)
	}
	pn.mu.Unlock()
	for _, peer := range peers {
		peer.Send(message)
	}
}

// RandomSend random send message to one peer
func (pn *Network) RandomSend(message proto.Message) {
	var peer *PeerSession
	pn.mu.Lock()
	if len(pn.peers) == 0 {
		return
	}
	ri := rand.Uint32() % uint32(len(pn.peers))
	var index uint32
	for _, p := range pn.peers {
		peer = p
		if index < ri {
			index++
			continue
		}
		break
	}
	pn.mu.Unlock()
	peer.Send(message)
}
