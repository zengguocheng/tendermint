package p2p

import (
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/tendermint/tendermint/crypto/ed25519"
)

func TestTransportAcceptDial(t *testing.T) {
	mt := NewMultiplexTransport(NodeKey{
		PrivKey: ed25519.GenPrivKey(),
	})

	addr, err := NewNetAddressStringWithOptionalID("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	err = mt.Listen(*addr)
	if err != nil {
		t.Fatal(err)
	}

	errc := make(chan error)

	go func() {
		dialer := NewMultiplexTransport(NodeKey{
			PrivKey: ed25519.GenPrivKey(),
		})

		addr, err := NewNetAddressStringWithOptionalID(mt.listener.Addr().String())
		if err != nil {
			errc <- err
			return
		}

		_, err = dialer.Dial(*addr, peerConfig{})
		if err != nil {
			errc <- err
			return
		}

		close(errc)
	}()

	if err := <-errc; err != nil {
		t.Errorf("connection failed: %v", err)
	}

	p, err := mt.Accept(peerConfig{})
	if err != nil {
		t.Fatal(err)
	}

	err = p.Start()
	if err != nil {
		t.Fatal(err)
	}

	err = p.Stop()
	if err != nil {
		t.Fatal(err)
	}

	if have, want := len(mt.peers), 0; have != want {
		t.Errorf("have %v, want %v", have, want)
	}
}

func TestTransportHandshake(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	peerNodeInfo := NodeInfo{}

	go func() {
		c, err := net.Dial(ln.Addr().Network(), ln.Addr().String())
		if err != nil {
			t.Error(err)
			return
		}

		go func(c net.Conn) {
			_, err := cdc.MarshalBinaryWriter(c, peerNodeInfo)
			if err != nil {
				t.Error(err)
			}
		}(c)
		go func(c net.Conn) {
			ni := NodeInfo{}

			_, err := cdc.UnmarshalBinaryReader(
				c,
				&ni,
				int64(MaxNodeInfoSize()),
			)
			if err != nil {
				t.Error(err)
			}
		}(c)
	}()

	c, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}

	ni, err := handshake(c, 20*time.Millisecond, NodeInfo{})
	if err != nil {
		t.Fatal(err)
	}

	if have, want := ni, peerNodeInfo; !reflect.DeepEqual(have, want) {
		t.Errorf("have %v, want %v", have, want)
	}
}
