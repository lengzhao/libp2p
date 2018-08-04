package libp2p

import (
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/lengzhao/libp2p/crypto"
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
	listener   net.Listener
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
	rand.Read(key)
	for k, v := range pn.keygen {
		pn.selfKeygen = k
		pn.selfKey = v.GetPrivKey(key)
		break
	}
	pn.publicKey = pn.selfKey.GetPublic()
	pn.address = fmt.Sprintf("kcp://%x@%s", pn.publicKey, pn.addr.String())
}

type pkgConn struct{ *net.UDPConn }

func (c *pkgConn) WriteTo(b []byte, addr net.Addr) (int, error) { return c.Write(b) }

type clientConn struct {
	conn      *net.UDPConn
	rAddr     *net.UDPAddr
	rAddrStr  string
	rdChan    chan []byte
	die       chan bool
	cache     []byte
	rdTimeout time.Duration
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
	log.Printf("client write to %s,len:%d\n", c.rAddr.String(), len(b))
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
		return err
	}
	//pn.listener, err = kcp.ServeConn(nil, 0, 0, pn.UDPConn)
	pn.listener, err = kcp.ServeConn(nil, 0, 0, pn)
	if err != nil {
		return err
	}

	fmt.Println("Listen address:", pn.address, pn.listener)
	for {
		conn, err := pn.listener.Accept()
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
	pn.mu.Unlock()
	if ok {
		if len(session.peerID) == 0 {
			session.peerID = id
		}
		return session
	}
	raddr, _ := net.ResolveUDPAddr("udp", u.Host)
	c := newClientConn(pn.UDPConn, raddr)
	pn.clientRCV[raddr.String()] = c
	conn, err := kcp.NewConn(u.Host, nil, 0, 0, c)
	if err != nil {
		return nil
	}

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
	rcv.rdTimeout = time.Duration(time.Now().Add(time.Second * 5).UnixNano())
	pn.timeWait = append(pn.timeWait, rcv)
	log.Printf("network add timewait:%p, peer:%s\n", pn, session.remoteAddr)
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
		log.Printf("Network receive data.net:%p, from:%s, len:%d\n", pn, addr.String(), n)
		pn.mu.Lock()
		pn.checkTimeWait()
		rcv, ok := pn.clientRCV[addr.String()]
		pn.mu.Unlock()
		if !ok {
			break
		}
		buff := make([]byte, n)
		copy(buff, b)
		select {
		case <-rcv.die:
			//closed,drop the data
			continue
		case rcv.rdChan <- buff:
			log.Printf("client receive data.from:%s, len:%d \n", addr.String(), n)
		}
	}

	return
}
