package p2p

import (
	"errors"
	"net"
	"time"

	"github.com/tendermint/tendermint/config"
	crypto "github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/p2p/conn"
)

// accept is the container to carry the upgrade connection and nodeInfo from
// asynchronously running routine to the Accept method.
type accept struct {
	conn     net.Conn
	nodeInfo NodeInfo
	err      error
}

// peerConfig is used to bundle data we need to fully setup a Peer with an
// MConn, provided by the caller of Accept and Dial.
type peerConfig struct {
	chDescs              []*conn.ChannelDescriptor
	onPeerError          func(Peer, interface{})
	outbound, persistent bool
	reactorsByCh         map[byte]Reactor
}

// Transport emits and connects to Peers. The implementation of Peer is left to
// the transport. Each transport is also responsible to filter establishing
// peers specific to its domain.
type Transport interface {
	// Accept returns a newly connected Peer.
	Accept(peerConfig) (Peer, error)

	// Dial connects to the Peer for the address.
	Dial(NetAddress, peerConfig) (Peer, error)
}

// transportLifecycle bundles the methods for callers to control start and stop
// behaviour.
type transportLifecycle interface {
	Close() error
	Listen(NetAddress) error
}

// multiplexTransport accepts and dials tcp connections and upgrades them to
// multiplexed peers.
type multiplexTransport struct {
	listener net.Listener

	acceptc chan accept
	closec  chan struct{}

	// Lookup table for duplicate ip and id checks.
	peers map[net.Conn]NodeInfo

	dialTimeout      time.Duration
	handshakeTimeout time.Duration
	nodeAddr         NetAddress
	nodeInfo         NodeInfo
	nodeKey          NodeKey

	// TODO(xla): Those configs are still needed as we parameterise peerConn and
	// peer currently. All relevant options should be refactored into options with
	// sane defaults.
	mConfig   conn.MConnConfig
	p2pConfig config.P2PConfig
}

// Test multiplexTransport for interface completeness.
var _ Transport = (*multiplexTransport)(nil)
var _ transportLifecycle = (*multiplexTransport)(nil)

// NewMultiplexTransport returns a tcp conencted multiplexed peers.
func NewMultiplexTransport() *multiplexTransport {
	return &multiplexTransport{
		acceptc: make(chan accept),
		closec:  make(chan struct{}),
	}
}

func (mt *multiplexTransport) Accept(cfg peerConfig) (Peer, error) {
	select {
	case a := <-mt.acceptc:
		if a.err != nil {
			return nil, a.err
		}

		err := mt.filterPeer(a.conn, a.nodeInfo)
		if err != nil {
			return nil, errors.New("peer filtered")
		}

		cfg.outbound = false

		return mt.wrapPeer(a.conn, a.nodeInfo, cfg), nil
	case <-mt.closec:
		return nil, errors.New("transport is closed")
	}
}

func (mt *multiplexTransport) Dial(
	addr NetAddress,
	cfg peerConfig,
) (Peer, error) {
	c, err := addr.DialTimeout(mt.dialTimeout)
	if err != nil {
		return nil, err
	}

	uc, ni, err := upgrade(c, mt.handshakeTimeout, mt.nodeKey.PrivKey, mt.nodeInfo)
	if err != nil {
		return nil, errors.New("upgrade failed")
	}

	err = mt.filterPeer(uc, ni)
	if err != nil {
		return nil, errors.New("peer filtered")
	}

	cfg.outbound = true

	return mt.wrapPeer(uc, ni, cfg), nil
}

func (mt *multiplexTransport) Close() error {
	close(mt.closec)

	return mt.listener.Close()
}

func (mt *multiplexTransport) Listen(addr NetAddress) error {
	ln, err := net.Listen("tcp", addr.DialString())
	if err != nil {
		return err
	}

	mt.listener = ln
	mt.nodeAddr = addr

	go mt.acceptPeers()

	return nil
}

func (mt *multiplexTransport) acceptPeers() {
	for {
		c, err := mt.listener.Accept()
		if err != nil {
			// Close has been called so we silently exit.
			if _, ok := <-mt.closec; !ok {
				return
			}

			mt.acceptc <- accept{err: err}
			return
		}

		go func(conn net.Conn) {
			c, ni, err := upgrade(
				c,
				mt.handshakeTimeout,
				mt.nodeKey.PrivKey,
				mt.nodeInfo,
			)

			select {
			case mt.acceptc <- accept{c, ni, err}:
				// Make the upgraded peer available.
			case <-mt.closec:
				// Give up if the transport was closed.
				_ = c.Close()
				return
			}
		}(c)
	}
}

func (mt *multiplexTransport) filterPeer(c net.Conn, ni NodeInfo) error {
	// TODO(xla): Implement IP filter.
	// TODO(xla): Implement ID filter.
	// TODO(xla): Check NodeInfo compatibility.
	return nil
}

func (mt *multiplexTransport) wrapPeer(
	c net.Conn,
	ni NodeInfo,
	cfg peerConfig,
) Peer {
	pc := peerConn{
		conn:       c,
		config:     &mt.p2pConfig,
		outbound:   cfg.outbound,
		persistent: cfg.persistent,
	}

	mt.peers[c] = ni

	return &multiplexPeer{
		onStop: func() {
			delete(mt.peers, c)
		},
		peer: newPeer(
			pc,
			mt.mConfig,
			ni,
			cfg.reactorsByCh,
			cfg.chDescs,
			cfg.onPeerError,
		),
	}
}

func handshake(
	c net.Conn,
	timeout time.Duration,
	nodeInfo NodeInfo,
) (NodeInfo, error) {
	if err := c.SetDeadline(time.Now().Add(timeout)); err != nil {
		return NodeInfo{}, err
	}

	var (
		errc = make(chan error, 2)

		peerNodeInfo NodeInfo
	)

	go func(errc chan<- error, c net.Conn) {
		_, err := cdc.MarshalBinaryWriter(c, nodeInfo)
		errc <- err
	}(errc, c)
	go func(errc chan<- error, c net.Conn) {
		_, err := cdc.UnmarshalBinaryReader(
			c,
			&peerNodeInfo,
			int64(MaxNodeInfoSize()),
		)
		errc <- err
	}(errc, c)

	for i := 0; i < cap(errc); i++ {
		err := <-errc
		if err != nil {
			return NodeInfo{}, err
		}
	}

	return peerNodeInfo, c.SetDeadline(time.Time{})
}

func secretConn(
	c net.Conn,
	timeout time.Duration,
	privKey crypto.PrivKey,
) (net.Conn, error) {
	if err := c.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}

	c, err := conn.MakeSecretConnection(c, privKey)
	if err != nil {
		return nil, err
	}

	return c, c.SetDeadline(time.Time{})
}

func upgrade(
	conn net.Conn,
	timeout time.Duration,
	privKey crypto.PrivKey,
	nodeInfo NodeInfo,
) (sc net.Conn, ni NodeInfo, err error) {
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()

	c, err := secretConn(conn, timeout, privKey)
	if err != nil {
		return nil, NodeInfo{}, err
	}

	ni, err = handshake(c, timeout, nodeInfo)
	if err != nil {
		return nil, NodeInfo{}, err
	}

	return c, ni, nil
}

// multiplexPeer is the Peer implementation returned by the multiplexTransport
// and wraps the default peer implementation for now, with an added hook for
// shutdown of the a peer. It should ultimately grow into the proper
// implementation.
type multiplexPeer struct {
	*peer
	onStop func()
}

func (mp *multiplexPeer) OnStop() {
	mp.onStop()
	mp.peer.OnStop()
}
