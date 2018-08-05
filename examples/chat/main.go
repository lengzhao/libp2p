package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/lengzhao/libp2p"
	"github.com/lengzhao/libp2p/dht"
	"github.com/lengzhao/libp2p/examples/chat/msg"
	"log"
	"os"
	"time"
)

// ChatPlugin chat plugin
type ChatPlugin struct {
	*libp2p.Plugin
}

// Receive Receive message
func (state *ChatPlugin) Receive(ctx *libp2p.PluginContext) error {
	switch info := ctx.GetMessage().(type) {
	case *msg.Message:
		fmt.Printf("Info <%s> %d %s \n", ctx.Session.GetRemoteAddress(), len(info.Data), info.Data)
	}

	return nil
}

//first node: ./chat.exe
//second node: ./chat.exe -addr=kcp://127.0.0.1:3002 -peer=kcp://9a422e34d4f8@127.0.0.1:3000
//the peer address is the listen address of first node
func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	address := flag.String("addr", "kcp://127.0.0.1:3000", "listen address")
	peer := flag.String("peer", "", "peer address")

	flag.Parse()
	n := libp2p.NewNetwork(*address)
	if n == nil {
		fmt.Println("error address")
		os.Exit(2)
	}

	n.AddPlugin(new(dht.DiscoveryPlugin))
	n.AddPlugin(new(ChatPlugin))
	if *peer != "" {
		go func() {
			time.Sleep(1 * time.Second)
			n.Bootstrap(*peer)
		}()
	}
	go func() {
		//fmt.Printf("Please enter your message: ")
		var info string
		inputReader := bufio.NewReader(os.Stdin)
		for {
			//fmt.Scan(&info)
			info, _ = inputReader.ReadString('\n')
			if len(info) == 0 {
				continue
			}
			fmt.Println("my send:", len(info), info)
			n.Broadcast(&msg.Message{Data: info})
		}

	}()
	n.Listen()
}
