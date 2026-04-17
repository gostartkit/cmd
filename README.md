# cmd
a very simple and modern command lib for Go

### Features
- Instance-driven (App struct) or Global state (DefaultApp)
- context.Context support in Run
- Recursive subcommands
- Long and short flags
- Global flags before commands, such as `app --verbose version`
- Interspersed flags, such as `app sayhi abc -v`
- Command suggestions (Levenshtein distance)
- Flag suggestions for unknown options
- Built-in help and usage generation
- Flag metadata: required, hidden, deprecated, category, example
- Environment variable binding for flags
- Optional JSON config file support with built-in `--config`
- Built-in shell completion for bash, zsh, fish, and powershell
- Built-in machine-readable `spec` export in JSON
- Enum and dynamic value completion for flag values
- Positionals can be documented, validated, exported, and completed

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
		f.SetCategory("force", "Global")
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

### flag metadata
```go
app.EnableConfigSupport()

cmdHello := &cmd.Command{
	Name:      "hello",
	UsageLine: "hello [flags] <target>",
	Short:     "print greeting",
	Examples: []string{
		"app hello team --name sam",
		"APP_NAME=sam app hello user",
	},
	Positionals: []cmd.PositionalArg{
		{Name: "target", Usage: "greeting target", Required: true, Enum: []string{"team", "user"}, Example: "team"},
	},
	SetFlags: func(f *cmd.FlagSet) {
		f.StringVar(&_name, "name", "", "target name", "n")
		f.BindEnv("name", "APP_NAME")
		f.BindConfig("name", "name")
		f.SetEnum("name", "sam", "sara")
		f.MarkRequired("name")
		f.SetCategory("name", "Input")
		f.SetExample("name", "sam")
	},
	Run: runHello,
}
```

### config precedence
```bash
app --config app.json hello
APP_NAME=sam app --config app.json hello
app --config app.json hello --name cli
```

The effective order is: CLI flag > env > config > default.

### positional arguments
```go
cmdDeploy := &cmd.Command{
	Name:      "deploy",
	UsageLine: "deploy <env> [service]",
	Positionals: []cmd.PositionalArg{
		{Name: "env", Required: true, Enum: []string{"dev", "staging", "prod"}},
		{
			Name: "service",
			Completion: func(ctx cmd.CompletionContext) []string {
				return []string{"api", "worker", "web"}
			},
		},
	},
}
```

### value completion
```go
f.StringVar(&_format, "format", "", "output format", "f")
f.SetEnum("format", "json", "yaml", "text")

f.StringVar(&_name, "name", "", "target name", "n")
f.SetCompletion("name", func(ctx cmd.CompletionContext) []string {
	return []string{"sam", "sara", "tom"}
})
```

### shell completion
```bash
app completion bash > /etc/bash_completion.d/app
app completion zsh > "${fpath[1]}/_app"
app completion fish > ~/.config/fish/completions/app.fish
```

### machine-readable spec
```bash
app spec
app spec json
```
