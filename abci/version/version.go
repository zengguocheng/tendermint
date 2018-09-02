package version

import "fmt"

// NOTE: we should probably be versioning the ABCI and the abci-cli separately

const Maj = "0"
const Min = "12"
const Fix = "0"
const Meta = ""

//const Version = "0.12.0"

var Version = func() string {
	v := fmt.Sprintf("%s.%s.%s", Maj, Min, Fix)
	if Meta != "" {
		v += "-" + Meta
	}
	return v
}()
