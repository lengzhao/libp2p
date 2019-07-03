package plugins

import (
	"time"

	"github.com/lengzhao/libp2p"
)

// Bootstrap bootstrap plugin
type Bootstrap struct {
	*libp2p.Plugin
	addrs []string
}

// NewBootstrap Bootstrap will create new session after networn startup
func NewBootstrap(addrs []string) *Bootstrap {
	out := new(Bootstrap)
	out.addrs = addrs
	return out
}

// Startup is called only once when the plugin is loaded
func (d *Bootstrap) Startup(net libp2p.Network) {
	go func() {
		time.Sleep(2 * time.Second)
		for _, addr := range d.addrs {
			s, err := net.NewSession(addr)
			if err != nil {
				continue
			}
			s.Send(Ping{})
		}
	}()
}
