package conn

import (
	"bytes"
	"errors"
	"log"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/lengzhao/libp2p"
)

// S2SPool udp server to udp server.
type S2SPool struct {
	mu      sync.Mutex
	active  bool
	scheme  string
	address *dfAddr
	server  net.PacketConn
	conns   map[string]*s2sConn
	handle  func(libp2p.Conn)
}

var closeData = []byte("_close")
var keepaliveData = []byte("_alive")

const (
	connOpsClose = iota
	connOpsKeepalive
	connOpsData
)

// Listen listen
func (c *S2SPool) Listen(addr string, handle func(libp2p.Conn)) error {
	if c.active {
		return errors.New("error status,it is actived")
	}
	u, err := url.Parse(addr)
	if err != nil {
		return err
	}

	c.server, err = net.ListenPacket("udp", u.Host)
	if err != nil {
		return err
	}
	u.Host = c.server.LocalAddr().String()

	c.scheme = u.Scheme
	c.address = newAddr(u, true)
	c.active = true
	c.conns = make(map[string]*s2sConn)
	c.handle = handle
	log.Println("listen:", c.address.String())
	for {
		data := make([]byte, 1500)
		n, peer, err := c.server.ReadFrom(data)
		if err != nil {
			return nil
		}
		pAddr := peer.String()
		var ops = connOpsData
		if n == len(closeData) && bytes.Compare(data[:n], closeData) == 0 {
			ops = connOpsClose
		} else if n == len(keepaliveData) && bytes.Compare(data[:n], keepaliveData) == 0 {
			ops = connOpsKeepalive
		}
		c.mu.Lock()
		conn, ok := c.conns[pAddr]
		c.mu.Unlock()
		switch ops {
		case connOpsClose:
			if ok {
				c.mu.Lock()
				delete(c.conns, pAddr)
				c.mu.Unlock()
				conn.Close()
			}
		case connOpsKeepalive:
			if ok {
				conn.rcount = 0
			}
		case connOpsData:
			if !ok {
				p, _ := url.Parse(addr)
				p.Host = peer.String()
				p.User = nil
				conn = newS2SConn(c, newAddr(p, false), peer)
				c.mu.Lock()
				old := c.conns[pAddr]
				c.conns[pAddr] = conn
				c.mu.Unlock()
				if old != nil {
					old.Close()
				}
				go handle(conn)
			}
			conn.rcount = 0
			conn.cache(data[:n])
		}
	}
}

// Dial dial
func (c *S2SPool) Dial(addr string) (libp2p.Conn, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	if u.User == nil || u.User.Username() == "" {
		return nil, errors.New("unknow user id")
	}
	if !c.active {
		// log.Println("s2s server is not active,dial udp:", addr)
		conn, err := net.Dial("udp", u.Host)
		if err != nil {
			return nil, err
		}
		out := new(udpClient)
		out.Conn = conn
		out.peerAddr = newAddr(u, true)
		out.timeout = defaultTimeout
		u2, _ := url.Parse(addr)
		u2.Host = conn.LocalAddr().String()
		u2.User = nil
		u2.Scheme = "udp"
		out.selfAddr = newAddr(u2, false)
		return out, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	conn, ok := c.conns[u.Host]
	if ok {
		return conn, nil
	}

	nConn, err := net.Dial("udp", u.Host)
	if err != nil {
		return nil, err
	}
	out := newS2SConn(c, newAddr(u, true), nConn.RemoteAddr())
	c.conns[u.Host] = out
	return out, nil
}

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (c *S2SPool) Close() {
	c.server.Close()
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, conn := range c.conns {
		delete(c.conns, k)
		go conn.Close()
	}
	return
}

func (c *S2SPool) removeConn(addr string) {
	c.mu.Lock()
	delete(c.conns, addr)
	c.mu.Unlock()
}

type s2sConn struct {
	mu     sync.Mutex
	conn   *S2SPool
	buff   []byte
	cached chan []byte

	peer     net.Addr
	selfAddr *dfAddr
	peerAddr *dfAddr
	rcount   int
	die      chan bool
}

func newS2SConn(p *S2SPool, paddr *dfAddr, udpAddr net.Addr) *s2sConn {
	out := new(s2sConn)
	out.cached = make(chan []byte, 100)
	out.conn = p
	out.selfAddr = p.address
	out.peerAddr = paddr
	out.peer = udpAddr
	out.die = make(chan bool)
	time.AfterFunc(defaultTimeout/3, out.timeout)
	return out
}

func (c *s2sConn) cache(data []byte) {
	select {
	case <-c.die:
	case c.cached <- data:
	default:
	}
}

func (c *s2sConn) timeout() {
	select {
	case <-c.die:
		return
	default:
		c.Write(keepaliveData)
	}
	if c.rcount > 3 {
		c.Close()
		return
	}
	c.rcount++
	time.AfterFunc(defaultTimeout/3, c.timeout)
}

// Read reads data from the connection.
// Read can be made to time out and return an Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (c *s2sConn) Read(b []byte) (n int, err error) {
	// log.Println("start to read:", len(b), c.LocalAddr().String())
	if c.buff == nil {
		select {
		case <-c.die:
		case c.buff = <-c.cached:
		}
	}
	if c.buff == nil {
		return 0, errors.New("the connect is closed. c.buff == nil")
	}
	copy(b, c.buff)
	n = len(b)
	if n >= len(c.buff) {
		n = len(c.buff)
		c.buff = nil
	} else {
		c.buff = c.buff[n:]
	}

	return
}

// Write writes data to the connection.
// Write can be made to time out and return an Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetWriteDeadline.
func (c *s2sConn) Write(b []byte) (n int, err error) {
	return c.conn.server.WriteTo(b, c.peer)
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (c *s2sConn) Close() error {
	select {
	case <-c.die:
	default:
		close(c.die)
		c.Write(closeData)
		go c.conn.removeConn(c.peer.String())
	}

	return nil
}

func (c *s2sConn) RemoteAddr() libp2p.Addr {
	return c.peerAddr
}

func (c *s2sConn) LocalAddr() libp2p.Addr {
	return c.selfAddr
}
