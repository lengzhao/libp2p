package kcp

import (
	"encoding/binary"
	"errors"
	//kcp "github.com/xtaci/kcp-go"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

// Listener listener
type Listener struct {
	laddr      *net.UDPAddr
	mu         sync.Mutex
	peers      map[string]*Connect
	conn       *net.UDPConn
	network    string
	address    string
	chAccepts  chan *Connect
	die        chan bool
	beListened bool
}

// Connect Connect
type Connect struct {
	l       *Listener
	raddr   *net.UDPAddr
	kcp     *KCP
	conv    uint32
	mu      sync.Mutex
	reading sync.Mutex
	writing sync.Mutex

	in         chan int
	out        chan int
	receiveNum int
	sendNum    int

	isClosed bool
	die      chan bool

	buffer []byte
}

const (
	errBrokenPipe       = "broken pipe"
	errInvalidOperation = "invalid operation"
)

// NewListener new kcp listener
func NewListener(network, addr string) (*Listener, error) {
	l := new(Listener)
	var err error
	l.laddr, err = net.ResolveUDPAddr(network, addr)
	if err != nil {
		log.Println("error address.", network, addr, err)
		return nil, err
	}
	l.peers = make(map[string]*Connect)
	l.network = network
	l.die = make(chan bool)
	l.chAccepts = make(chan *Connect)
	return l, nil
}

// Listen listen
func (l *Listener) Listen() (err error) {
	l.conn, err = net.ListenUDP(l.network, l.laddr)
	if err != nil {
		log.Println("fail to listen:", l.network, l.laddr.String())
		return err
	}
	l.address = l.laddr.String()
	l.beListened = true
	go func() {
		for {
			buff := make([]byte, 1500)
			n, raddr, err := l.conn.ReadFromUDP(buff)
			if err != nil {
				log.Println("listen read error:", err)
				break
			}
			addrStr := raddr.String()
			l.mu.Lock()
			conn, ok := l.peers[addrStr]
			conv := binary.BigEndian.Uint32(buff)
			if !ok {
				// remote start the connection
				conn = l.newConnect(conv, raddr)
				l.peers[addrStr] = conn
				l.chAccepts <- conn
			} else if conv != conn.conv {
				// for NAT Traversal,
				// 1. a ----> NAT --x-> b,  (a use conv1,b do not receive it)
				// 2. b ----> NAT ----> a,  (b use conv2)
				if conn.receiveNum == 0 {
					conn.Close()
					conn = l.newConnect(conv, raddr)
					l.peers[addrStr] = conn
					l.chAccepts <- conn
				}
			}
			l.mu.Unlock()
			conn.mu.Lock()
			if conn.isClosed {
				conn.mu.Unlock()
				continue
			}
			log.Println("kcp receive:", n, addrStr, time.Now().String())
			conn.kcp.Input(buff[:n])
			conn.receiveNum++
			conn.mu.Unlock()
			conn.in <- n
		}
	}()
	return
}

// NewConnect new connection
func (l *Listener) NewConnect(addr string) *Connect {
	raddr, err := net.ResolveUDPAddr(l.network, addr)
	if err != nil {
		return nil
	}
	conv := rand.Uint32()
	conn := l.newConnect(conv, raddr)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.peers[raddr.String()] = conn
	return conn
}

func (l *Listener) newConnect(conv uint32, raddr *net.UDPAddr) *Connect {
	conn := new(Connect)
	conn.kcp = NewKCP(conv, conn.output)
	conn.kcp.Update()
	conn.in = make(chan int, 10)
	conn.out = make(chan int, 10)
	conn.die = make(chan bool)
	conn.conv = conv
	conn.l = l
	conn.raddr = raddr
	return conn
}

// Accept implements the Accept method in the Listener interface; it waits for the next call and returns a generic Conn.
func (l *Listener) Accept() (net.Conn, error) {
	return l.AcceptKCP()
}

// AcceptKCP accepts a KCP connection
func (l *Listener) AcceptKCP() (*Connect, error) {
	select {
	case c := <-l.chAccepts:
		return c, nil
	case <-l.die:
		return nil, errors.New(errBrokenPipe)
	}
}

func (l *Listener) removeConnect(conn *Connect) {
	time.Sleep(10 * time.Second)
	key := conn.raddr.String()
	l.mu.Lock()
	defer l.mu.Unlock()
	peer, ok := l.peers[key]
	if !ok || peer.conv != conn.conv {
		return
	}
	delete(l.peers, key)
}

// Close close
func (l *Listener) Close() error {
	select {
	case <-l.die:
		return errors.New(errInvalidOperation)
	default:
		close(l.die)
		if l.beListened {
			l.conn.Close()
		}

		l.mu.Lock()
		defer l.mu.Unlock()
		for k, peer := range l.peers {
			peer.Close()
			delete(l.peers, k)
		}
	}
	return nil
}

func (c *Connect) output(buf []byte) {
	// c.l.conn.WriteToUDP(buf[:size], c.raddr)
	// c.sendEvent <- true
	n, err := c.l.conn.WriteToUDP(buf, c.raddr)
	log.Println("write data to ", c.raddr.String(), n, err, time.Now().String())
}

func (c *Connect) Read(buf []byte) (n int, err error) {
	c.reading.Lock()
	defer c.reading.Unlock()
	if c.isClosed {
		return 0, errors.New(errBrokenPipe)
	}
	n = c.readFromBuff(buf)
	if n > 0 {
		return
	}
	n = c.readFromKCP(buf)
	if n > 0 {
		return n, nil
	}
	for {
		select {
		case <-c.in:
			n = c.readFromKCP(buf)
			if n > 0 {
				return n, nil
			}
		case <-c.die:
			return 0, errors.New("broken pipe")
		}
	}
}

func (c *Connect) readFromBuff(buf []byte) (n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.buffer == nil {
		return 0
	}
	copy(buf, c.buffer)
	if len(buf) > len(c.buffer) {
		n = len(c.buffer)
		c.buffer = nil
	} else {
		n = len(buf)
		c.buffer = c.buffer[n:]
	}
	return
}

func (c *Connect) readFromKCP(buf []byte) (n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var err error
	c.buffer = make([]byte, CMaxMtuSize)
	n, err = c.kcp.Read(c.buffer)
	if err != nil || n == 0 {
		return 0
	}
	c.buffer = c.buffer[:n]

	copy(buf, c.buffer)
	if len(buf) < n {
		n = len(buf)
	}
	c.buffer = c.buffer[n:]
	return n
}

func (c *Connect) Write(b []byte) (n int, err error) {
	if c.isClosed {
		return 0, errors.New(errBrokenPipe)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		n, err = c.kcp.Write(b)
		if err != nil {
			return
		}
		if n != 0 {
			break
		}
		time.Sleep(time.Duration(cMinInterval))
	}
	return
}

// Close close connection
func (c *Connect) Close() error {
	if c.isClosed {
		return errors.New(errBrokenPipe)
	}
	select {
	case <-c.die:
		return errors.New(errBrokenPipe)
	default:
		close(c.die)
		c.isClosed = true
		go c.l.removeConnect(c)
	}
	return nil
}

// LocalAddr get local address
func (c *Connect) LocalAddr() net.Addr {
	return c.l.laddr
}

// RemoteAddr get remote address
func (c *Connect) RemoteAddr() net.Addr {
	return c.raddr
}

// SetDeadline set deadline
func (c *Connect) SetDeadline(t time.Time) error { return nil }

// SetReadDeadline set read deadline
func (c *Connect) SetReadDeadline(t time.Time) error { return nil }

// SetWriteDeadline set write deadline
func (c *Connect) SetWriteDeadline(t time.Time) error { return nil }

// GetConv get conv
func (c *Connect) GetConv() uint32 {
	return c.conv
}
