package network

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"log"
	"runtime/debug"
	"sync"

	"github.com/lengzhao/libp2p"
)

// Session session
type Session struct {
	mgr      *Manager
	conn     libp2p.Conn
	scheme   string
	id       int
	mu       sync.Mutex
	peerAddr string
	peerID   []byte
	selfID   []byte
	env      sync.Map
}

const (
	magic = 21341
)

func newSession(m *Manager, conn libp2p.Conn, peer []byte, sync bool) *Session {
	out := new(Session)
	out.conn = conn
	out.scheme = m.scheme
	out.mgr = m
	out.selfID = m.cryp.GetPublic()
	out.peerID = peer

	if len(peer) > 0 {
		conn.RemoteAddr().UpdateUser(hex.EncodeToString(peer))
		for _, p := range m.plugins {
			p.PeerConnect(out)
		}
	}

	if sync {
		out.receive()
	} else {
		go out.receive()
	}

	return out
}

type dataHeader struct {
	Magic uint16
	Len   uint16
}

func (s *Session) receive() {
	defer func() {
		// log.Println("session close:", s.conn.RemoteAddr().String())
		s.conn.Close()
		if len(s.peerID) == 0 {
			return
		}
		for _, p := range s.mgr.plugins {
			p.PeerDisconnect(s)
		}
	}()
	for s.mgr.active {
		headBuf := make([]byte, 1500)
		n, err := s.conn.Read(headBuf)
		if err != nil {
			// log.Printf("conn(peer:%s) read err:%s\n", s.conn.RemoteAddr().String(), err)
			return
		}
		headBuf = headBuf[:n]
		var head dataHeader
		buf := bytes.NewReader(headBuf)
		binary.Read(buf, binary.BigEndian, &head)
		if head.Magic != magic {
			// log.Println("error magic of message:", head.Magic)
			continue
		}
		if head.Len == 0 {
			continue
		}
		data := make([]byte, head.Len)
		copy(data, headBuf[4:])
		var offset uint16
		offset = uint16(len(headBuf)) - 4
		for offset < head.Len {
			n, err := s.conn.Read(data[offset:])
			if err != nil {
				// log.Println("fail to read message:", err)
				return
			}
			offset += uint16(n)
		}
		if len(data) == 0 {
			continue
		}
		signLen := data[0]
		if len(data) <= int(signLen)+5 {
			continue
		}
		sign := data[1 : signLen+1]
		data = data[signLen+1:]
		err = s.process(data, sign)
		// if err != nil {
		// 	log.Println("fail to process:", err)
		// }
	}
}

// Send send struct data,encode by gob
func (s *Session) Send(msg interface{}) error {
	e := new(Event)
	if bytes.Compare(s.selfID, s.peerID) == 0 {
		return errors.New("unable send to self")
	}
	if len(s.peerID) == 0 {
		return errors.New("unknow peer id")
	}
	s.mu.Lock()
	s.id++
	if s.id == 0 {
		s.id++
	}
	e.ID = s.id
	e.SignType = s.mgr.cryp.GetType()
	e.From = s.selfID
	e.To = s.peerID
	e.Info = msg
	s.mu.Unlock()
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(e)
	if err != nil {
		// log.Println("fail to encode msg:", err)
		return err
	}
	data := buf.Bytes()
	if len(data) > 65000 {
		return errors.New("data too long")
	}
	sign := s.mgr.cryp.Sign(data)
	var head dataHeader
	head.Magic = magic
	head.Len = uint16(len(data) + len(sign) + 1)
	hbuf := new(bytes.Buffer)
	binary.Write(hbuf, binary.BigEndian, head)
	hbuf.WriteByte(byte(len(sign)))
	hbuf.Write(sign)
	buf.WriteTo(hbuf)
	hbuf.WriteTo(s.conn)
	//buf.WriteTo(s.conn)
	//log.Println("write len:", len(hbuf.Bytes()), len(data))
	return nil
}

func (s *Session) process(data, sign []byte) error {
	defer func() {
		err := recover()
		if err != nil {
			stat.Add("recover", 1)
			log.Println("[error]session process:", err)
			log.Println(string(debug.Stack()))
		}
	}()
	buf := bytes.NewReader(data)
	dec := gob.NewDecoder(buf)
	var e Event
	err := dec.Decode(&e)
	if err != nil {
		// log.Println("decode error:", err)
		stat.Add("err_event", 1)
		return err
	}
	e.session = s
	if bytes.Compare(s.selfID, e.To) != 0 {
		s.Close()
		// log.Printf("error self id:%x,hope:%x\n", e.To, s.selfID)
		stat.Add("err_sid", 1)
		return errors.New("error self id")
	}
	if len(s.peerID) == 0 {
		s.peerID = e.From
		s.GetPeerAddr().UpdateUser(hex.EncodeToString(e.From))
		for _, p := range s.mgr.plugins {
			p.PeerConnect(s)
		}
	}
	if bytes.Compare(s.peerID, e.From) != 0 {
		s.Close()
		stat.Add("err_pid", 1)
		return errors.New("error peer id")
	}
	// log.Printf("sign info.self:%x,sign:%x,peer:%x\n", s.selfID, sign, s.peerID)
	check := s.mgr.cryp.Verify(e.SignType, data, sign, s.peerID)
	if !check {
		s.Close()
		stat.Add("err_sign", 1)
		return errors.New("error sign")
	}
	stat.Add("event", 1)
	// log.Printf("event:%#v\n", e)
	for _, p := range s.mgr.plugins {
		p.Receive(&e)
	}
	return nil
}

// GetPeerAddr get remote address
func (s *Session) GetPeerAddr() libp2p.Addr {
	return s.conn.RemoteAddr()
}

// GetSelfAddr get self address
func (s *Session) GetSelfAddr() libp2p.Addr {
	return s.conn.LocalAddr()
}

// Close close session
func (s *Session) Close() {
	s.conn.Close()
}

// GetNetwork get network
func (s *Session) GetNetwork() libp2p.Network {
	return s.mgr
}

// SetEnv set env
func (s *Session) SetEnv(key, value string) {
	s.env.Store(key, value)
}

// GetEnv get env
func (s *Session) GetEnv(key string) string {
	v, ok := s.env.Load(key)
	if ok {
		return v.(string)
	}
	return ""
}
