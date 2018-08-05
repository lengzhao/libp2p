package main

import (
	"flag"
	"fmt"
	"github.com/lengzhao/libp2p"
	"github.com/lengzhao/libp2p/dht"
	"log"
	"os"
	"time"
)

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
	if *peer != "" {
		go func() {
			time.Sleep(1 * time.Second)
			n.Bootstrap(*peer)
		}()
	}
	n.Listen()
}
