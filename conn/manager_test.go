package conn

import (
	"errors"
	"github.com/lengzhao/libp2p"
	"log"
	"testing"
	"time"
)

type Handler1 struct {
	Data chan []byte
}

func (h *Handler1) cb(conn libp2p.Conn) {
	data := make([]byte, 100)
	n, _ := conn.Read(data)
	h.Data <- data[:n]
	conn.Close()
}

func runCloseByServer(addr1, addr2 string) error {
	h := new(Handler1)
	h.Data = make(chan []byte)
	mgr1 := new(PoolMgr)
	mgr1.pool = make(map[string]libp2p.ConnPool)
	mgr1.RegConnPool("tcp", new(TCPPool))
	mgr1.RegConnPool("udp", new(UDPPool))
	mgr1.RegConnPool("ws", new(WSPool))
	mgr1.RegConnPool("s2s", new(S2SPool))

	mgr2 := new(PoolMgr)
	mgr2.pool = make(map[string]libp2p.ConnPool)
	mgr2.RegConnPool("tcp", new(TCPPool))
	mgr2.RegConnPool("udp", new(UDPPool))
	mgr2.RegConnPool("ws", new(WSPool))
	mgr2.RegConnPool("s2s", new(S2SPool))
	go mgr1.Listen(addr1, h.cb)
	go mgr2.Listen(addr2, h.cb)

	defer mgr1.Close()
	defer mgr2.Close()
	time.Sleep(2 * time.Second)
	s1, err := mgr2.Dial(addr1)
	if err != nil {
		return err
	}
	defer s1.Close()
	data := "aaaaaa"
	s1.Write([]byte(data))
	serData := <-h.Data
	if data != string(serData) {
		return errors.New("fail to send data")
	}
	d := make([]byte, 10)
	n, err := s1.Read(d)
	if err == nil {
		log.Println("fail to read data,", string(d[:n]), n)
		return errors.New("fail to read data,hope return error")
	}

	return nil
}

type Handler2 struct {
	Data chan []byte
}

func (h *Handler2) cb(conn libp2p.Conn) {
	data := make([]byte, 100)
	n, _ := conn.Read(data)
	if n == 0 {
		h.Data <- nil
	} else {
		h.Data <- data[:n]
	}

	n, _ = conn.Read(data)
	if n == 0 {
		h.Data <- nil
	} else {
		h.Data <- data[:n]
	}

}

func runCloseByClient(addr1, addr2 string) error {
	h := new(Handler2)
	h.Data = make(chan []byte)
	mgr1 := new(PoolMgr)
	mgr1.pool = make(map[string]libp2p.ConnPool)
	mgr1.RegConnPool("tcp", new(TCPPool))
	mgr1.RegConnPool("udp", new(UDPPool))
	mgr1.RegConnPool("ws", new(WSPool))
	mgr1.RegConnPool("s2s", new(S2SPool))

	mgr2 := new(PoolMgr)
	mgr2.pool = make(map[string]libp2p.ConnPool)
	mgr2.RegConnPool("tcp", new(TCPPool))
	mgr2.RegConnPool("udp", new(UDPPool))
	mgr2.RegConnPool("ws", new(WSPool))
	mgr2.RegConnPool("s2s", new(S2SPool))
	go mgr1.Listen(addr1, h.cb)
	go mgr2.Listen(addr2, h.cb)

	defer mgr1.Close()
	defer mgr2.Close()
	time.Sleep(2 * time.Second)
	s1, err := mgr2.Dial(addr1)
	if err != nil {
		return err
	}
	//defer s1.Close()
	data := "aaaaa"
	s1.Write([]byte(data))
	serData := <-h.Data
	if data != string(serData) {
		return errors.New("fail to send data")
	}
	s1.Close()
	serData = <-h.Data
	if serData != nil {
		return errors.New("fail to close connection,"+string(serData))
	}

	return nil
}

func TestManager1(t *testing.T) {
	addr1 := "ws://addr1@127.0.0.1:3000/echo"
	addr2 := "ws://addr2@127.0.0.1:3001/echo"
	err := runCloseByServer(addr1, addr2)
	if err != nil {
		t.Error(err)
	}
	err = runCloseByClient(addr1, addr2)
	if err != nil {
		t.Error(err)
	}
}
func TestManager2(t *testing.T) {
	addr1 := "udp://addr1@127.0.0.1:3000/echo"
	addr2 := "udp://addr2@127.0.0.1:3001/echo"
	err := runCloseByServer(addr1, addr2)
	if err != nil {
		t.Error(err)
	}
	err = runCloseByClient(addr1, addr2)
	if err != nil {
		t.Error(err)
	}
}
func TestManager3(t *testing.T) {
	addr1 := "s2s://addr1@127.0.0.1:3000/echo"
	addr2 := "s2s://addr2@127.0.0.1:3001/echo"
	err := runCloseByServer(addr1, addr2)
	if err != nil {
		t.Error(err)
	}
	err = runCloseByClient(addr1, addr2)
	if err != nil {
		t.Error(err)
	}
}
func TestManager4(t *testing.T) {
	addr1 := "tcp://addr1@127.0.0.1:3000/echo"
	addr2 := "tcp://addr2@127.0.0.1:3001/echo"
	err := runCloseByServer(addr1, addr2)
	if err != nil {
		t.Error(err)
	}
	err = runCloseByClient(addr1, addr2)
	if err != nil {
		t.Error(err)
	}
}

// udp client io timeout
// func TestManager5(t *testing.T) {
// 	addr1 := "udp://addr1@127.0.0.1:3000/echo"
// 	addr2 := "udp://addr2@127.0.0.1:3001/echo"

// 	h := new(Handler2)
// 	h.Data = make(chan []byte)
// 	mgr1 := new(PoolMgr)
// 	mgr1.pool = make(map[string]libp2p.ConnPool)
// 	mgr1.RegConnPool("tcp", new(TCPPool))
// 	mgr1.RegConnPool("udp", new(UDPPool))
// 	mgr1.RegConnPool("ws", new(WSPool))
// 	mgr1.RegConnPool("s2s", new(S2SPool))

// 	mgr2 := new(PoolMgr)
// 	mgr2.pool = make(map[string]libp2p.ConnPool)
// 	mgr2.RegConnPool("tcp", new(TCPPool))
// 	mgr2.RegConnPool("udp", new(UDPPool))
// 	mgr2.RegConnPool("ws", new(WSPool))
// 	mgr2.RegConnPool("s2s", new(S2SPool))
// 	go mgr1.Listen(addr1, h.cb)
// 	go mgr2.Listen(addr2, h.cb)

// 	defer mgr1.Close()
// 	defer mgr2.Close()
// 	time.Sleep(2 * time.Second)
// 	s1, err := mgr2.Dial(addr1)
// 	if err != nil {
// 		t.Error(err)
// 		return
// 	}
// 	data := "aaaaa"
// 	s1.Write([]byte(data))
// 	serData := <-h.Data
// 	if data != string(serData) {
// 		t.Error("fail to send data")
// 		return
// 	}

// 	buf := make([]byte, 10)
// 	_, err = s1.Read(buf)
// 	if err == nil {
// 		t.Error("hope io timeout")
// 		return
// 	}
// 	log.Println("read restult:", err)
// 	s1.Close()

// 	serData = <-h.Data
// 	if serData != nil {
// 		t.Error("fail to close connection")
// 		return
// 	}
// }
