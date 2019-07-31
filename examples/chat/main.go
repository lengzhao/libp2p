package main

import (
	"bufio"
	"encoding/gob"
	"flag"
	"github.com/lengzhao/libp2p"
	"github.com/lengzhao/libp2p/network"
	"github.com/lengzhao/libp2p/plugins"
	"log"
	"os"
	"strings"
)

// ChatPlugin chat plugin
type ChatPlugin struct {
	*libp2p.Plugin
}

// Message chat message
type Message struct {
	Data string
}

type interMsg struct {
	typ string
	msg interface{}
}

func (m *interMsg) GetType() string {
	return m.typ
}
func (m *interMsg) GetMsg() interface{} {
	return m.msg
}

func init() {
	gob.Register(Message{})
}

// Receive Receive message
func (state *ChatPlugin) Receive(e libp2p.Event) error {
	switch info := e.GetMessage().(type) {
	case Message:
		log.Printf("Info <%s> %d %s \n", e.GetSession().GetPeerAddr(), len(info.Data), info.Data)
		if info.Data == "request" {
			e.Reply(Message{Data: "response"})
		}
	}

	return nil
}

//first node: ./chat.exe
//second node: ./chat.exe -addr=kcp://127.0.0.1:3002 -peer=kcp://9a422e34d4f8@127.0.0.1:3000
//the peer address is the listen address of first node
func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	address := flag.String("addr", "tcp://127.0.0.1:3003", "listen address")
	peer := flag.String("peer", "tcp://47ed0566bff0@127.0.0.1:3000", "peer address")

	flag.Parse()
	n := network.New()
	if n == nil {
		log.Println("error address")
		os.Exit(2)
	}

	n.RegistPlugin(new(plugins.DiscoveryPlugin))
	n.RegistPlugin(plugins.NewBootstrap([]string{*peer}))
	n.RegistPlugin(plugins.NewBroadcast(0))
	n.RegistPlugin(new(ChatPlugin))
	go func() {
		//log.Printf("Please enter your message: ")
		var info string
		inputReader := bufio.NewReader(os.Stdin)
		for {
			//fmt.Scan(&info)
			info, _ = inputReader.ReadString('\n')
			info = strings.TrimSpace(info)
			if len(info) == 0 {
				continue
			}
			log.Println("my send:", len(info), info)
			msg := interMsg{"broadcast", Message{Data: info}}
			n.SendInternalMsg(&msg)
		}
	}()
	n.Listen(*address)
}
