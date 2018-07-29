package kcp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"testing"
	"time"
)

type TSender struct {
	k1 *KCP
	k2 *KCP
}

func TestNewKCP(t *testing.T) {
	sd := new(TSender)
	k1 := NewKCP(100, func(data []byte) {
		sd.k2.Input(data)
	})
	k2 := NewKCP(100, func(data []byte) {
		sd.k1.Input(data)
	})
	sd.k1 = k1
	sd.k2 = k2

	data := "send data 1"
	_, err := k1.Write([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 100)
	n, err := k2.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != data {
		t.Fatalf("error data:%s", buf[:n])
	}
}

func TestNewKCP2(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:12345")
	if err != nil {
		t.Fatal(err)
	}
	var dumpFile string
	listen, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer listen.Close()
	go func() {
		peers := make(map[string]*KCP)
		for {
			buff := make([]byte, 1500)
			n, raddr, err := listen.ReadFromUDP(buff)
			if err != nil {
				break
			}
			peer, ok := peers[raddr.String()]
			conv := binary.BigEndian.Uint32(buff)
			fn := fmt.Sprintf("receive_%d.data", conv)
			if !ok {
				peer = NewKCP(conv, func(data []byte) {
					listen.WriteTo(data, raddr)
				})
				peers[raddr.String()] = peer
				os.Remove(fn)
				dumpFile = fn
			}
			peer.Input(buff[:n])
			f, err := os.OpenFile(fn, os.O_CREATE|os.O_RDWR, 666)
			if err != nil {
				continue
			}
			f.Seek(0, 2)
			for {
				peer.Update()
				buff = make([]byte, 1500)
				n, err = peer.Read(buff)
				if err != nil || n <= 0 {
					break
				}
				f.Write(buff[:n])
			}
			f.Close()
			peer.Update()
		}
	}()

	client, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatal(err)
	}

	var index = 433
	k := NewKCP(54321, func(data []byte) {
		index *= 263
		if index%100 > 90 {
			fmt.Println("drop a package")
			return
		}
		client.Write(data)
	})
	k.SetMTU(400)
	wData, err := ioutil.ReadFile("kcp.go")
	if err != nil {
		t.Fatal(err)
	}
	k.Write(wData)
	go func() {
		time.Sleep(8 * time.Second)
		client.Close()
	}()
	rChan := make(chan []byte)
	go func() {
		for {

			buff := make([]byte, 100)
			n, err := client.Read(buff)
			if err != nil {
				break
			}
			if n > 0 {
				rChan <- buff[:n]
			}
		}
	}()
	die := make(chan bool)
	go func() {
		var count int
		for {
			select {
			case <-time.After(time.Millisecond * 10):
				k.Update()
				count++
				if count > 100 {
					die <- true
					return
				}
			case data := <-rChan:
				k.Input(data)
				count = 0
			}
		}
	}()
	<-die
	listen.Close()
	recData, _ := ioutil.ReadFile(dumpFile)
	if bytes.Compare(recData, wData) != 0 {
		t.Fatal("receive data != write data")
	}
	os.Remove(dumpFile)
}
