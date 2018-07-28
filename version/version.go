package version

var (
	// Version is the current software version of Tendermint Core.
	// It follows SemVer on all external interfaces (CLI, RPC, ABCI),
	// but not on the Golang APIs.
	Version string = "0.23.0"

	// GitCommit is the current HEAD set using ldflags.
	GitCommit string

	// BlockVersion is the blockchain Protocol Version.
	// It must be incremented every time there is a change
	// to the blockchain data structures.
	BlockVersion int64 = 13

	// P2PVersion is the p2p and reactors Protocol Version.
	// It must be incremented every time there is a change
	// to any of the p2p or reactor protocols - eg. any new
	// message type, change to a message type, or change to
	// expected behaviour.
	P2PVersion int64 = 8
)

// ProtocolVersion contains the current and proposed
// next versions of a protocol. It can be used to signal
// and trigger upgrades.
type ProtocolVersion struct {
	Current int64 `json:"current"`
	Next    int64 `json:"next"`
}

func init() {
	if GitCommit != "" {
		Version += "-" + GitCommit
	}
}
