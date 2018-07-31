package libp2p

// IPlugin is used to proxy callbacks to a particular Plugin instance.
type IPlugin interface {
	// Callback for when the network starts listening for peers.
	Startup(net *Network)

	// Callback for when the network stops listening for peers.
	Cleanup(net *Network)

	// Callback for when an incoming message is received. Return true
	// if the plugin will intercept messages to be processed.
	Receive(ctx *PluginContext) error

	// Callback for when a peer connects to the network.
	PeerConnect(s *PeerSession)

	// Callback for when a peer disconnects from the network.
	PeerDisconnect(s *PeerSession)
}

// Plugin is an abstract class which all plugins extend.
type Plugin struct{}

// Hook callbacks of network builder plugins

// Startup is called only once when the plugin is loaded
func (*Plugin) Startup(net *Network) {}

// Cleanup is called only once after network stops listening
func (*Plugin) Cleanup(net *Network) {}

// Receive is called every time when messages are received
func (*Plugin) Receive(ctx *PluginContext) error { return nil }

// PeerConnect is called every time a PeerSession is initialized and connected
func (*Plugin) PeerConnect(s *PeerSession) {}

// PeerDisconnect is called every time a PeerSession connection is closed
func (*Plugin) PeerDisconnect(s *PeerSession) {}
