package libp2p

import (
	"encoding/binary"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/lengzhao/libp2p/crypto"
	"github.com/xtaci/smux"
	"log"
	"net"
	// "runtime/debug"
	"time"
)

const (
	messageLengthLimit = 65000
)

// PeerSession peer session.
type PeerSession struct {
	Net        *Network
	session    *smux.Session
	peerID     []byte
	isServer   bool
	remoteAddr string
	die        chan bool
}

func newSession(n *Network, conn net.Conn, server bool) *PeerSession {
	session := new(PeerSession)
	var err error
	if server {
		session.session, err = smux.Server(conn, nil)
	} else {
		session.session, err = smux.Client(conn, nil)
	}
	if err != nil {
		log.Println("fail to new smux session,", err)
		return nil
	}
	session.Net = n
	session.isServer = server
	session.die = make(chan bool)
	rAddr := conn.RemoteAddr().String()
	n.mu.Lock()
	n.peers[rAddr] = session
	n.mu.Unlock()
	session.remoteAddr = rAddr
	log.Println("new connection:", rAddr, server)
	go session.receiveMsg()
	return session
}

// Close Close
func (c *PeerSession) Close() {
	select {
	case <-c.die:
		return
	default:
		close(c.die)
		c.session.Close()
		c.Net.closeSession(c)
	}
}

func (c *PeerSession) receiveMsg() {
	defer c.Close()
	for _, plugin := range c.Net.pulgins {
		plugin.PeerConnect(c)
		defer plugin.PeerDisconnect(c)
	}
	for {
		ns, err := c.session.AcceptStream()
		if err != nil {
			log.Println("fail to AcceptStream", c.Net.address, err)
			break
		}
		go c.process(ns)
	}
}

func (c *PeerSession) process(ns *smux.Stream) {
	// defer func() {
	// 	defer ns.Close()
	// 	if err := recover(); err != nil {
	// 		fmt.Println("process painc:", err)
	// 		debug.PrintStack()
	// 	}
	// }()
	data := make([]byte, binary.MaxVarintLen64)
	_, err := ns.Read(data)
	if err != nil {
		log.Println("fail to read from stream:", ns.ID())
		return
	}

	l, _ := binary.Varint(data)
	if l == 0 || l > messageLengthLimit {
		log.Println("error data length:", l)
		return
	}
	data = make([]byte, l)
	var n int
	for {
		ln, err := ns.Read(data)
		n += ln
		if err != nil || n >= int(l) {
			break
		}
	}
	if n < int(l) {
		log.Println("error data length:", n, "<", l)
		return
	}
	log.Println("receive data form:", c.GetRemoteAddress(), len(data))
	newContext(c, data)
}

// Send send message
func (c *PeerSession) Send(message proto.Message) error {
	any, err := types.MarshalAny(message)
	if err != nil {
		return fmt.Errorf("fail to MarshalAny")
	}
	msg := crypto.Message{DataMsg: any}
	data, err := proto.Marshal(&msg)
	if err != nil {
		return fmt.Errorf("fail to MarshalAny")
	}
	if data == nil {
		return fmt.Errorf("fail to sign")
	}
	l := int64(len(data))
	if l > messageLengthLimit {
		return fmt.Errorf("data too long:%d", l)
	}
	stream, err := c.session.OpenStream()
	if err != nil {
		return err
	}
	//defer stream.Close()
	stream.SetDeadline(time.Now().Add(10 * time.Second))

	ld := make([]byte, binary.MaxVarintLen64)
	binary.PutVarint(ld, l)
	_, err = stream.Write(ld)
	if err != nil {
		return err
	}
	_, err = stream.Write(data)
	log.Println("send data length:", l, ", stream id:", stream.ID())
	return err
}

// GetRemoteAddress get remote address. kcp://publicKey@host:port
func (c *PeerSession) GetRemoteAddress() string {
	addr := fmt.Sprintf("kcp://%x@%s", c.peerID, c.remoteAddr)
	return addr
}
