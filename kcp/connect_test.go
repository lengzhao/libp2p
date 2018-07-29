package kcp

import (
	"fmt"
	"testing"
	"time"
)

func TestNewListener(t *testing.T) {
	l, err := NewListener("udp", "127.0.0.1:0")
	if err != nil {
		t.Error(err)
		return
	}
	if l == nil {
		t.Error("empty listen")
		return
	}
	err = l.Close()
	if err != nil {
		t.Error(err)
	}
	err = l.Close()
	if err == nil {
		t.Error("close twice,hope error")
	}
}

func echoNode(addr string) *Listener {
	l, err := NewListener("udp", addr)
	if err != nil {
		return nil
	}
	err = l.Listen()
	if err != nil {
		l.Close()
		return nil
	}
	go func() {
		for {
			conn, err := l.AcceptKCP()
			if err != nil {
				break
			}
			buf := make([]byte, 256)
			n, err := conn.Read(buf)
			if err != nil {
				break
			}
			fmt.Printf("server receive.from:%s, data:%s\n", conn.RemoteAddr().String(), buf[:n])
			conn.Write(buf[:n])
		}
	}()
	return l
}

func TestNewConnect(t *testing.T) {
	node1 := echoNode("127.0.0.1:3000")
	node2 := echoNode("127.0.0.1:3001")
	if node1 == nil || node2 == nil {
		t.Fatal("empty node")
	}
	conn := node2.NewConnect("127.0.0.1:3000")
	if conn == nil {
		t.Fatal("fail to create connection")
	}
	fmt.Println("start send", time.Now().String())
	_, err := conn.Write([]byte("data1"))
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if "data1" != string(buf[:n]) {
		t.Fatal("not equal data")
	}
	fmt.Println("client receive:", string(buf[:n]), time.Now().String())
	conn.Close()
	node1.Close()
	node2.Close()
	//t.Error("stop")
}
