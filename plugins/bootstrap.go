package plugins

import (
	"time"

	"github.com/lengzhao/libp2p"
)

// Bootstrap bootstrap plugin
type Bootstrap struct {
	*libp2p.Plugin
	Addrs []string
}

// Startup is called only once when the plugin is loaded
func (d *Bootstrap) Startup(net libp2p.Network) {
	go func() {
		time.Sleep(2 * time.Second)
		for _, addr := range d.Addrs {
			s, err := net.NewSession(addr)
			if err != nil {
				continue
			}
			s.Send(Ping{IsServer: s.GetSelfAddr().IsServer()})
		}
	}()
}
