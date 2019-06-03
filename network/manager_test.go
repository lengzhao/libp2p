package network

import (
	"encoding/gob"
	"errors"
	"log"
	"testing"
	"time"

	"github.com/lengzhao/libp2p"
	"github.com/lengzhao/libp2p/plugins"
)

type Inner struct {
	Test int
}
type Inner2 struct {
	Test int
}

type Plugin struct {
	*libp2p.Plugin
	Inner int
	Other int
}

func (p *Plugin) Receive(e libp2p.Event) error {
	log.Printf("Plugin:%#v\n", e)

	switch msg := e.GetMessage().(type) {
	case Inner:
		log.Println("receive Inner event:", msg.Test)
		e.Reply(Inner2{3})
		p.Inner++
	default:
		log.Printf("receive unknow event:%#v\n", msg)
		p.Other++
	}
	return nil
}

func runTest(addr1, addr2 string) error {
	gob.Register(Inner{})
	gob.Register(Inner2{})
	mgr1 := New()
	defer mgr1.Close()
	mgr2 := New()
	defer mgr2.Close()

	plugin1 := &Plugin{}
	plugin2 := &Plugin{}
	mgr1.RegistPlugin(plugin1)
	mgr2.RegistPlugin(plugin2)
	go mgr1.Listen(addr1)
	go mgr2.Listen(addr2)
	for !mgr1.active || !mgr2.active {
		time.Sleep(time.Millisecond * 10)
	}
	addr := mgr1.GetAddress()
	log.Println("client address:", addr)
	s, err := mgr2.NewSession(addr)
	if err != nil {
		return err
	}

	defer s.Close()
	err = s.Send(Inner{2})
	if err != nil {
		return err
	}
	time.Sleep(2 * time.Second)

	if plugin1.Inner != 1 || plugin2.Other != 1 {
		log.Printf("error status,inner:%d,other:%d\n", plugin1.Inner, plugin2.Other)
		return errors.New("error status")
	}
	log.Println("-----------------finish-------------------")
	return nil
}

func TestNew(t *testing.T) {
	log.Println("start test:", t.Name())
	addr1 := "tcp://addr1@127.0.0.1:8081"
	addr2 := "tcp://addr2@127.0.0.1:8082"
	err := runTest(addr1, addr2)
	if err != nil {
		t.Error("error:", err)
	}
}

func TestNew2(t *testing.T) {
	log.Println("start test:", t.Name())
	addr1 := "udp://addr1@127.0.0.1:8081"
	addr2 := "udp://addr2@127.0.0.1:8082"
	err := runTest(addr1, addr2)
	if err != nil {
		t.Error("error:", err)
	}

}

func TestNew3(t *testing.T) {
	log.Println("start test:", t.Name())
	addr1 := "ws://addr1@127.0.0.1:8081/echo"
	addr2 := "ws://addr2@127.0.0.1:8082/echo"
	err := runTest(addr1, addr2)
	if err != nil {
		t.Error("error:", err)
	}
}

func TestNew4(t *testing.T) {
	log.Println("start test:", t.Name())
	addr1 := "s2s://addr1@127.0.0.1:8081"
	addr2 := "s2s://addr2@127.0.0.1:8082"
	err := runTest(addr1, addr2)
	if err != nil {
		t.Error("error:", err)
	}
}

type ConnCount struct {
	*libp2p.Plugin
	ConnNum    int
	DisconnNum int
}

// PeerConnect is called every time a Session is initialized and connected
func (p *ConnCount) PeerConnect(s libp2p.Session) {
	log.Println("new connection:", s.GetSelfAddr(), s.GetPeerAddr())
	s.SetEnv("inDHT", "true")
	p.ConnNum++
}

// PeerDisconnect is called every time a Session connection is closed
func (p *ConnCount) PeerDisconnect(s libp2p.Session) {
	// log.Println("disconnect:", s.GetSelfAddr(), s.GetPeerAddr())
	p.DisconnNum++
}

// func (p *ConnCount) Receive(e libp2p.Event) error {
// 	switch msg := e.GetMessage().(type) {
// 	default:
// 		log.Printf("event:%T,selfID:%s,peerID:%x,peerAddr%s\n",
// 			msg, e.GetSession().GetSelfAddr().User(),
// 			e.GetPeerID(), e.GetSession().GetSelfAddr())
// 	}
// 	return nil
// }

func runTest2(addrs ...string) error {
	gob.Register(Inner{})
	gob.Register(Inner2{})

	var mgr *Manager
	var plugin *ConnCount
	for _, addr := range addrs {
		m := New()
		defer m.Close()
		p := new(ConnCount)
		m.RegistPlugin(p)
		m.RegistPlugin(new(plugins.DiscoveryPlugin))
		m.RegistPlugin(plugins.NewBroadcast(0))
		go m.Listen(addr)
		for !m.active {
			time.Sleep(time.Millisecond * 10)
		}
		if mgr != nil {
			time.Sleep(time.Millisecond * 100)
			addr1 := mgr.GetAddress()
			s, err := m.NewSession(addr1)
			if err != nil {
				log.Println("fail to new session.", m.GetAddress(), "->", addr1)
				return err
			}
			defer s.Close()
			// err = s.Send(plugins.Ping{FromAddr: s.GetSelfAddr().String()})
			err = s.Send(Inner{2})
			if err != nil {
				log.Println("fail to send.", s.GetSelfAddr(), "->", s.GetPeerAddr())
				return err
			}
		}
		mgr = m
		plugin = p
	}
	time.Sleep(2 * time.Second)

	if plugin.ConnNum != len(addrs)-1 {
		log.Printf("error status,conn:%d,disconn:%d,hope:%d\n", plugin.ConnNum, plugin.DisconnNum, len(addrs)-1)
		return errors.New("error status")
	}
	log.Println("-----------------finish-------------------")
	return nil
}

func TestNew5(t *testing.T) {
	log.Println("start test:", t.Name())
	addr1 := "s2s://addr1@127.0.0.1:8081"
	addr2 := "s2s://addr2@127.0.0.1:8082"
	addr3 := "s2s://addr3@127.0.0.1:8083"
	addr4 := "s2s://addr4@127.0.0.1:8084"
	addr5 := "s2s://addr5@127.0.0.1:8085"
	err := runTest2(addr1, addr2, addr3, addr4, addr5)
	// err := runTest2(addr1, addr2, addr3)
	if err != nil {
		t.Error("error:", err)
	}
	// t.Error("error:")
}

func TestNew6(t *testing.T) {
	log.Println("start test:", t.Name())
	addr1 := "s2s://addr1@127.0.0.1:8081"
	addr2 := "s2s://addr2@127.0.0.1:8082"
	addr3 := "udp://addr3@127.0.0.1:8083"
	addr4 := "tcp://addr4@127.0.0.1:8084"
	addr5 := "ws://addr5@127.0.0.1:8085/test6"
	err := runTest2(addr1, addr2, addr3, addr4, addr5)
	if err != nil {
		t.Error("error:", err)
	}
	err = runTest2(addr1, addr2, addr5, addr4, addr3)
	if err != nil {
		t.Error("error:", err)
	}
	err = runTest2(addr1, addr2, addr5, addr3, addr4)
	if err != nil {
		t.Error("error:", err)
	}
	err = runTest2(addr2, addr5, addr3, addr4, addr1)
	if err != nil {
		t.Error("error:", err)
	}
}
