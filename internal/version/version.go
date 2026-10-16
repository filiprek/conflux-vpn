package version

import (
	"fmt"
	"runtime"
)

var Version = "dev"
var Commit = "unknown"

func String() string {
	return fmt.Sprintf("Conflux %s commit=%s go=%s protocol=1", Version, Commit, runtime.Version())
}
