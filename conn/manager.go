package conn

import (
	"errors"
	"github.com/lengzhao/libp2p"
	"log"
	"net"
	"net/url"
	"sync"
)

// PoolMgr pool manager
type PoolMgr struct {
	mu       sync.Mutex
	pool     map[string]libp2p.ConnPool
	handle   func(conn libp2p.Conn)
	listener libp2p.ConnPool
}

// NewMgr new manager of connection pool
func NewMgr() *PoolMgr {
	out := new(PoolMgr)
	out.pool = make(map[string]libp2p.ConnPool)
	return out
}

// GetDefaultMgr create default manager,support tcp,udp,ws,s2s
func GetDefaultMgr() *PoolMgr {
	out := NewMgr()
	out.RegConnPool("tcp", new(TCPPool))
	out.RegConnPool("udp", new(UDPPool))
	out.RegConnPool("ws", new(WSPool))
	out.RegConnPool("s2s", new(S2SPool))
	return out
}

// RegConnPool register new connection pool
func (mgr *PoolMgr) RegConnPool(scheme string, c libp2p.ConnPool) {
	mgr.mu.Lock()
	mgr.pool[scheme] = c
	mgr.mu.Unlock()
}

// Listen listen,the address must be with scheme,such as:"tcp://127.0.0.1:1111"
func (mgr *PoolMgr) Listen(addr string, handle func(libp2p.Conn)) error {
	u, err := url.Parse(addr)
	if err != nil {
		return err
	}
	pool, ok := mgr.pool[u.Scheme]
	if !ok {
		log.Println("fail to Listen.unknow scheme:", u.Scheme)
		return errors.New("unsupport scheme")
	}
	mgr.handle = handle
	mgr.listener = pool
	return pool.Listen(addr, mgr.process)
}

// Dial dial,the address must be with scheme,such as:"tcp://127.0.0.1:1111"
func (mgr *PoolMgr) Dial(addr string) (libp2p.Conn, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	pool, ok := mgr.pool[u.Scheme]
	if !ok {
		return nil, errors.New("unsupport scheme")
	}
	return pool.Dial(addr)
}

func (mgr *PoolMgr) process(conn libp2p.Conn) {
	defer func() {
		recover()
	}()
	mgr.handle(conn)
}

// Close close
func (mgr *PoolMgr) Close() {
	mgr.listener.Close()
}

type dfConn struct {
	net.Conn
	peerAddr *dfAddr
	selfAddr *dfAddr
}

func (c *dfConn) RemoteAddr() libp2p.Addr {
	return c.peerAddr
}

func (c *dfConn) LocalAddr() libp2p.Addr {
	return c.selfAddr
}

// dfAddr such as:  ws://user@ip:port/path
type dfAddr struct {
	addr     *url.URL
	isServer bool
}

func newAddr(addr *url.URL, isServer bool) *dfAddr {
	out := new(dfAddr)
	out.addr = addr
	out.isServer = isServer
	return out
}

func (a *dfAddr) String() string {
	return a.addr.String()
}

func (a *dfAddr) Scheme() string {
	return a.addr.Scheme
}

func (a *dfAddr) User() string {
	return a.addr.User.Username()
}
func (a *dfAddr) Host() string {
	return a.addr.Host
}
func (a *dfAddr) IsServer() bool {
	return a.isServer
}
func (a *dfAddr) UpdateUser(user string) {
	if a.addr.User == nil || a.addr.User.Username() == "" {
		a.addr.User = url.User(user)
	}
}
