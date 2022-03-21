package about

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
)

var (
	version     = "0.0.0"                // semantic version X.Y.Z
	gitCommit   = "00000000"             // sha1 from git
	buildTime   = "1970-01-01T00:00:00Z" // build date in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
	showVersion *bool
)

func AddFlags(fs *pflag.FlagSet) {
	showVersion = fs.Bool("version", false, "Display version info")
}

func DisplayAndExitIfRequested() {
	if *showVersion {
		DisplayVersion()
		os.Exit(0)
	}
}

func DisplayVersion() {
	fmt.Printf("Version: %s\nBuildTime: %s\nGitCommit: %s\n", version, buildTime, gitCommit)
}
