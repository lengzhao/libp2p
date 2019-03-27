package network

import (
	"encoding/gob"
	"errors"
	"github.com/lengzhao/libp2p"
	"log"
	"testing"
	"time"
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

func runTest(addr string) error {
	gob.Register(Inner{})
	gob.Register(Inner2{})
	mgr := New()
	defer mgr.Close()

	plugin := &Plugin{}
	mgr.RegistPlugin(plugin)
	go mgr.Listen(addr)
	for !mgr.active {
		time.Sleep(time.Millisecond * 10)
	}
	addr = mgr.GetAddress()
	s, err := mgr.NewSession(addr)
	if err != nil {
		return err
	}

	defer s.Close()
	err = s.Send(Inner{2})
	if err != nil {
		return err
	}
	time.Sleep(2 * time.Second)

	if plugin.Inner != 1 || plugin.Other != 1 {
		log.Printf("error status,inner:%d,other:%d\n", plugin.Inner, plugin.Other)
		return errors.New("error status")
	}
	log.Println("-----------------finish-------------------")
	return nil
}

func TestNew(t *testing.T) {
	log.Println("start test:", t.Name())
	addr := "tcp://127.0.0.1:8081"
	err := runTest(addr)
	if err != nil {
		t.Error("error:", err)
	}
}

func TestNew2(t *testing.T) {
	log.Println("start test:", t.Name())
	addr := "udp://127.0.0.1:8081"
	err := runTest(addr)
	if err != nil {
		t.Error("error:", err)
	}

}

func TestNew3(t *testing.T) {
	log.Println("start test:", t.Name())
	addr := "ws://127.0.0.1:8081/echo"
	err := runTest(addr)
	if err != nil {
		t.Error("error:", err)
	}
}

func TestNew4(t *testing.T) {
	log.Println("start test:", t.Name())
	addr := "s2s://127.0.0.1:8082"
	err := runTest(addr)
	if err != nil {
		t.Error("error:", err)
	}
}
