package conn

import (
	"log"
	"net/http"
	"net/url"

	"github.com/lengzhao/libp2p"
	"golang.org/x/net/websocket"
)

// WSPool websocker pool
type WSPool struct {
	server *http.Server
	cb     func(libp2p.Conn)
	addr   *dfAddr
}

// Listen listen
func (c *WSPool) Listen(addr string, handle func(libp2p.Conn)) error {
	u, err := url.Parse(addr)
	if err != nil {
		log.Println("fail to parse addr.", addr, err)
		return err
	}
	hander := http.NewServeMux()
	hander.Handle(u.Path, websocket.Handler(c.handler))

	c.server = &http.Server{Addr: u.Host, Handler: hander}
	c.cb = handle
	c.addr = newAddr(u, true)
	log.Println("listen:", c.addr.String())
	return c.server.ListenAndServe()
}

func (c *WSPool) handler(conn *websocket.Conn) {
	peer, _ := url.Parse(conn.RemoteAddr().String())
	peer.Scheme = c.addr.Scheme()
	peer.Path = "client"
	out := new(dfConn)
	out.Conn = conn
	out.peerAddr = newAddr(peer, false)
	out.selfAddr = c.addr
	c.cb(out)
}

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (c *WSPool) Close() {
	c.server.Close()
}

// Dial dial
func (c *WSPool) Dial(addr string) (libp2p.Conn, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	conn, err := websocket.Dial(addr, "", "http://"+u.Host)
	if err != nil {
		return nil, err
	}
	out := new(dfConn)
	out.Conn = conn
	out.peerAddr = newAddr(u, true)
	u2, _ := url.Parse(conn.LocalAddr().String())
	u2.Host = conn.LocalAddr().String()
	u2.Scheme = out.peerAddr.Scheme()
	u2.User = nil
	u2.Path = "client"
	out.selfAddr = newAddr(u2, false)
	return out, nil
}
