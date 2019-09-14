package libp2p

/*
 ------------------------------------------
    plugin1 -> plugin2 -> plugin3 ...
 ------------------------------------------
             Event
 ------------------------------------------
			Session
 ------------------------------------------
   TCP | UDP | Websocket | KCP | P2P ...
 ------------------------------------------
              OS
 ------------------------------------------
*/

// Addr the address of connection
type Addr interface {
	String() string
	Scheme() string
	User() string
	Host() string
	IsServer() bool
	UpdateUser(string)
	SetServer()
}

// Conn is a generic stream-oriented network connection.
//
// Multiple goroutines may invoke methods on a Conn simultaneously.
type Conn interface {
	// Read reads data from the connection.
	// Read can be made to time out and return an Error with Timeout() == true
	// after a fixed time limit; see SetDeadline and SetReadDeadline.
	Read(b []byte) (n int, err error)

	// Write writes data to the connection.
	// Write can be made to time out and return an Error with Timeout() == true
	// after a fixed time limit; see SetDeadline and SetWriteDeadline.
	Write(b []byte) (n int, err error)

	// Close closes the connection.
	// Any blocked Read or Write operations will be unblocked and return errors.
	Close() error

	// LocalAddr returns the local network address.
	LocalAddr() Addr

	// RemoteAddr returns the remote network address.
	RemoteAddr() Addr
}

// ConnPoolMgr manager of connection pool,address such as:"tcp://127.0.0.1:1111"
type ConnPoolMgr interface {
	RegConnPool(scheme string, c ConnPool)
	Listen(address string, handle func(conn Conn)) error
	Dial(address string) (Conn, error)
	// close the listener
	Close()
}

// ConnPool connection pool,address such as:"tcp://127.0.0.1:1111"
type ConnPool interface {
	Listen(address string, handle func(conn Conn)) error
	Dial(address string) (Conn, error)
	// close the listener
	Close()
}

// InterMsg internal message
type InterMsg interface {
	GetType() string
	GetMsg() interface{}
}

// Network interface of network manager
type Network interface {
	// address:  tcp://127.0.0.1:80
	NewSession(address string) (Session, error)
	RegistPlugin(IPlugin)
	SendInternalMsg(InterMsg)
	GetAddress() string
	Close()
}

// Session interface of session
type Session interface {
	// encode by gob
	Send(interface{}) error
	GetPeerAddr() Addr
	GetSelfAddr() Addr
	SetEnv(key, value string)
	GetEnv(key string) string
	Close()
}

// Event interface of event
type Event interface {
	// encode by gob
	Reply(interface{}) error
	GetMessage() interface{}
	GetSession() Session
	GetPeerID() []byte
}

// CryptoMgr crypto manager
type CryptoMgr interface {
	SetPrivKey(typ string, key []byte) error
	GetType() string
	Sign(data []byte) []byte
	Verify(typ string, data, sig, pubKey []byte) bool
	Register(k SignKey)
	GetPublic() []byte
}

// SignKey interface define of crypto key
type SignKey interface {
	GetType() string
	Verify(data, sig, pubKey []byte) bool
	// Cryptographically sign the given bytes
	Sign(data, privKey []byte) []byte
	// Return a public key paired with this private key
	GetPublic(privKey []byte) []byte
}

// IPlugin is used to proxy callbacks to a particular Plugin instance.
type IPlugin interface {
	// Callback for when the network starts listening for peers.
	Startup(net Network)

	// Callback for when the network stops listening for peers.
	Cleanup(net Network)

	// Callback for when an incoming message is received. Return true
	// if the plugin will intercept messages to be processed.
	Receive(e Event) error

	// RecInternalMsg receive internal message
	RecInternalMsg(InterMsg) error

	// Callback for when a peer connects to the network.
	PeerConnect(s Session)

	// Callback for when a peer disconnects from the network.
	PeerDisconnect(s Session)
}

// Plugin is an abstract class which all plugins extend.
type Plugin struct{}

// Startup is called only once when the plugin is loaded
func (*Plugin) Startup(net Network) {}

// Cleanup is called only once after network stops listening
func (*Plugin) Cleanup(net Network) {}

// Receive is called every time when messages are received
func (*Plugin) Receive(e Event) error { return nil }

// RecInternalMsg receive internal message
func (*Plugin) RecInternalMsg(InterMsg) error { return nil }

// PeerConnect is called every time a Session is initialized and connected
func (*Plugin) PeerConnect(s Session) {}

// PeerDisconnect is called every time a Session connection is closed
func (*Plugin) PeerDisconnect(s Session) {}
