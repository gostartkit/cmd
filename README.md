# cmd
a very simple and modern command lib for Go

### Features
- Instance-driven (App struct) or Global state (DefaultApp)
- context.Context support in Run
- Recursive subcommands
- Long and short flags
- Command suggestions (Levenshtein distance)
- Built-in help and usage generation

### main
```go
package main

import (
	"context"
	"pkg.gostartkit.com/cmd"
)

var (
	_webForce = false
)

func main() {

	cmd.SetFlags(func(f *cmd.FlagSet) {
		f.BoolVar(&_webForce, "force", false, "force operation", "f")
	})

	cmd.AddCommands(cmdVersion)
	cmd.Execute()
}
```

### cmdVersion
```go
package main

import (
	"context"
	"fmt"
	"pkg.gostartkit.com/cmd"
)

var (
	_version = "v1.0.0"
	_osarch  string // set by ldflags

	cmdVersion = &cmd.Command{
		Run:       runVersion,
		Name:      "version",
		UsageLine: "version",
		Short:     "display version",
		Long:      "display version and build info.\n",
	}
)

func runVersion(ctx context.Context, cmd *cmd.Command, args []string) error {
	fmt.Println(_version, _osarch)
	return nil
}
```
