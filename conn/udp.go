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

// UDPPool connection over udp,it could lose message.
type UDPPool struct {
	mu      sync.Mutex
	active  bool
	scheme  string
	address libp2p.Addr
	server  net.PacketConn
	conns   map[string]*udpConn
}

const defaultTimeout = time.Second * 20

// Listen listen
func (c *UDPPool) Listen(addr string, handle func(libp2p.Conn)) error {
	if c.active {
		return errors.New("error status,it is actived")
	}
	u, err := url.Parse(addr)
	if err != nil {
		return err
	}

	c.server, err = net.ListenPacket(u.Scheme, u.Host)
	if err != nil {
		return err
	}
	u.Host = c.server.LocalAddr().String()

	c.scheme = u.Scheme
	c.address = newAddr(u, true)
	c.active = true
	c.conns = make(map[string]*udpConn)
	log.Println("listen:", c.address.String())
	for {
		data := make([]byte, 1500)
		n, peer, err := c.server.ReadFrom(data)
		if err != nil {
			log.Println("fail to readFrom:", c.address.String(), err)
			if !c.active {
				return nil
			}
			continue
		}
		address := peer.String()
		if n == len(closeData) && bytes.Compare(data[:n], closeData) == 0 {
			c.mu.Lock()
			conn, ok := c.conns[address]
			if ok {
				delete(c.conns, address)
			}
			c.mu.Unlock()
			if ok {
				conn.Close()
			}
			continue
		}
		c.mu.Lock()
		conn, ok := c.conns[address]
		if !ok {
			conn = newUDPConn(c, peer)
			c.conns[address] = conn
			go handle(conn)
		}
		c.mu.Unlock()
		// log.Println("receive:", address, string(data[:n]))
		conn.cache(data[:n])
	}
}

type udpClient struct {
	dfConn
	timeout time.Duration
	wmu     sync.Mutex
}

func (c *udpClient) Close() error {
	c.Conn.Write(closeData)
	return c.Conn.Close()
}

func (c *udpClient) Read(data []byte) (int, error) {
	c.Conn.SetReadDeadline(time.Now().Add(c.timeout))
	n, err := c.Conn.Read(data)
	if err != nil {
		return 0, err
	}
	if n == len(closeData) && bytes.Compare(data[:n], closeData) == 0 {
		defer c.Conn.Close()
		return 0, errors.New("closed")
	}
	return n, err
}

// Write writes data to the connection.
// Write can be made to time out and return an Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetWriteDeadline.
func (c *udpClient) Write(b []byte) (n int, err error) {
	var dfNum int = UDPMtu
	var num int
	c.wmu.Lock()
	defer c.wmu.Unlock()
	for len(b) > 0 {
		if len(b) < dfNum {
			dfNum = len(b)
		}
		num, err = c.Conn.Write(b[:dfNum])
		b = b[dfNum:]
		if err != nil {
			return
		}
		n += num
	}
	return
}

// Dial dial
func (c *UDPPool) Dial(addr string) (libp2p.Conn, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial(u.Scheme, u.Host)
	if err != nil {
		return nil, err
	}
	out := new(udpClient)
	out.Conn = conn
	out.peerAddr = newAddr(u, true)
	u2, _ := url.Parse(addr)
	u2.Host = conn.LocalAddr().String()
	u2.User = nil
	out.selfAddr = newAddr(u2, false)
	out.timeout = defaultTimeout
	return out, nil
}

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (c *UDPPool) Close() {
	c.active = false
	c.server.Close()
	for k, conn := range c.conns {
		delete(c.conns, k)
		conn.Close()
	}
	return
}

func (c *UDPPool) removeConn(addr string) {
	c.mu.Lock()
	delete(c.conns, addr)
	c.mu.Unlock()
}

type udpConn struct {
	wmu      sync.Mutex
	active   bool
	conn     *UDPPool
	buff     []byte
	cached   chan []byte
	peer     net.Addr
	selfAddr libp2p.Addr
	peerAddr libp2p.Addr
	die      chan bool
	closed   bool
}

func newUDPConn(p *UDPPool, addr net.Addr) *udpConn {
	out := new(udpConn)
	out.cached = make(chan []byte, 100)
	out.conn = p
	out.peer = addr
	out.active = true
	out.die = make(chan bool)
	out.selfAddr = p.address
	pa := new(url.URL)
	pa.Scheme = p.address.Scheme()
	pa.Host = addr.String()
	out.peerAddr = newAddr(pa, false)
	return out
}

func (c *udpConn) cache(data []byte) {
	select {
	case <-c.die:
	case c.cached <- data:
	default:
	}
}

// Read reads data from the connection.
// Read can be made to time out and return an Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (c *udpConn) Read(b []byte) (n int, err error) {
	//log.Println("start to read:", len(b), c.LocalAddr().String())
	if c.buff == nil {
		to := make(chan int, 1)
		time.AfterFunc(defaultTimeout, func() { to <- 1 })
		select {
		case <-c.die:
		case c.buff = <-c.cached:
			// log.Println("read data:", c.LocalAddr().String())
		case <-to:
			// log.Println("read timeout:", c.LocalAddr().String())
			return 0, errors.New("read timeout")
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
func (c *udpConn) Write(b []byte) (n int, err error) {
	var dfNum int = UDPMtu
	var num int
	c.wmu.Lock()
	defer c.wmu.Unlock()
	for len(b) > 0 {
		if len(b) < dfNum {
			dfNum = len(b)
		}
		num, err = c.conn.server.WriteTo(b[:dfNum], c.peer)
		b = b[dfNum:]
		if err != nil {
			return
		}
		n += num
	}
	return
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (c *udpConn) Close() error {
	defer recover()
	c.active = false
	select {
	case <-c.die:
		return nil
	default:
		c.wmu.Lock()
		if c.closed {
			c.wmu.Unlock()
			return nil
		}
		c.closed = true
		c.wmu.Unlock()
		close(c.die)
		c.Write(closeData)
		// log.Println("close udpConn:", c.peer.String())
		c.conn.removeConn(c.peer.String())
	}

	return nil
}

// LocalAddr returns the local network address.
func (c *udpConn) LocalAddr() libp2p.Addr {
	return c.selfAddr
}

// RemoteAddr returns the remote network address.
func (c *udpConn) RemoteAddr() libp2p.Addr {
	return c.peerAddr
}
