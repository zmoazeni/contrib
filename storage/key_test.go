package storage

import (
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/require"
)

func TestKey(t *testing.T) {
	a := require.New(t)
	var p Peer
	a.NoError(p.FromInputPeer(&tg.InputPeerUser{
		UserID:     10,
		AccessHash: 10,
	}))
	k := KeyFromPeer(p)

	data := k.Bytes(nil)
	a.NoError(k.Parse(data))
}
