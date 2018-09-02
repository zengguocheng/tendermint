package version

import "fmt"

// Version components
const (
	Maj = "0"
	Min = "23"
	Fix = "1"
	Meta = ""
)

var (
	// Version is the current version of Tendermint
	// Must be a string because scripts like dist.sh read this file.
	//Version = "0.23.1"

	// GitCommit is the current HEAD set using ldflags.
	GitCommit string
)

func init() {
	if GitCommit != "" {
		Version += "-" + GitCommit
	}
}

var Version = func () string {
	v := fmt.Sprintf("%s.%s.%s", Maj, Min, Fix)
	if Meta != "" {
		v += "-" + Meta
	}
	return v
}()
