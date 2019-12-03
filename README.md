# cmd
a very simple command lib

### main
```go
package main

import (
	"flag"

	"github.com/webpkg/cmd"
)

var (
	_webForce = false
)

func main() {

	cmd.SetFlags(func(f *flag.FlagSet) {
		f.BoolVar(&_webForce, "force", false, "")
	})

	cmd.AddCommands(cmdVersion)
	cmd.Execute()
}

```

### cmdVersion
```go
package main

import (
	"fmt"

	"github.com/webpkg/cmd"
)

var (
	_version = "v0.0.1"
	_osarch  string // set by ldflags

	cmdVersion = &cmd.Command{
		Run:       runVersion,
		UsageLine: "version",
		Short:     "display version",
		Long:      "display version and build info.\n",
	}
)

func runVersion(cmd *cmd.Command, args []string) {
	fmt.Println(_version, _osarch)
}

```