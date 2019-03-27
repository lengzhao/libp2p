package main

import (
	"flag"
	"github.com/lengzhao/libp2p/dht"
	"github.com/lengzhao/libp2p/network"
	"log"
	"os"
	"time"
)

func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	address := flag.String("addr", "tcp://127.0.0.1:3000", "listen address")
	peer := flag.String("peer", "", "peer address")

	flag.Parse()
	n := network.New()
	if n == nil {
		log.Println("error address")
		os.Exit(2)
	}

	n.RegistPlugin(new(dht.DiscoveryPlugin))
	if *peer != "" {
		go func() {
			time.Sleep(1 * time.Second)
			//n.Bootstrap(*peer)
		}()
	}
	n.Listen(*address)
}
