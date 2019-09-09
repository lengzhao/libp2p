package conn

import (
	"log"
	"net"
	"net/url"

	"github.com/lengzhao/libp2p"
)

// TCPPool default connection,such as:tcp,unix
type TCPPool struct {
	l    net.Listener
	addr *dfAddr
}

// Listen listen
func (c *TCPPool) Listen(addr string, handle func(libp2p.Conn)) error {
	u, err := url.Parse(addr)
	if err != nil {
		log.Println("fail to parse address.addr:", addr, err)
		return err
	}
	c.l, err = net.Listen(u.Scheme, u.Host)
	if err != nil {
		log.Println("fail to listen address.addr:", u.Scheme, u.Host, err)
		return err
	}
	u.Host = c.l.Addr().String()
	c.addr = newAddr(u, true)
	log.Println("listen:", c.addr.String())
	for {
		conn, err := c.l.Accept()
		if err != nil {
			log.Println("fail to accept new connection", err)
			return nil
		}
		u1, _ := url.Parse(addr)
		u1.Host = conn.LocalAddr().String()
		u2, _ := url.Parse(addr)
		u2.Host = conn.RemoteAddr().String()
		u2.User = nil
		out := new(dfConn)
		out.Conn = conn
		out.peerAddr = newAddr(u2, false)
		out.selfAddr = newAddr(u1, true)
		go handle(out)
	}
}

// Close close the listener
func (c *TCPPool) Close() {
	c.l.Close()
}

// Dial dial
func (c *TCPPool) Dial(addr string) (libp2p.Conn, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial(u.Scheme, u.Host)
	if err != nil {
		return nil, err
	}
	out := new(dfConn)
	out.Conn = conn
	out.peerAddr = newAddr(u, true)
	u2, _ := url.Parse(addr)
	u2.Host = conn.LocalAddr().String()
	u2.User = nil
	out.selfAddr = newAddr(u2, false)
	return out, nil
}
