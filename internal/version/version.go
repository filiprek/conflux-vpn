package version

import (
	"fmt"
	"runtime"
)

var Version = "0.1.0-dev"
var Commit = "unknown"

func String() string {
	return fmt.Sprintf("Conflux %s commit=%s go=%s protocol=1", Version, Commit, runtime.Version())
}
