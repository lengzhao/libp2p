package main

import (
	"flag"
	"github.com/lengzhao/libp2p/network"
	"github.com/lengzhao/libp2p/plugins"
	"log"
	"os"
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

	n.RegistPlugin(new(plugins.DiscoveryPlugin))
	n.RegistPlugin(new(plugins.Broadcast))
	bs := new(plugins.Bootstrap)
	bs.Addrs = []string{*peer}
	n.RegistPlugin(bs)
	n.Listen(*address)
}
