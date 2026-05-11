# cmd

English | [简体中文](./README.zh-CN.md)

`cmd` is a modern command-line library for Go. It keeps a small API surface, but adds the capabilities usually needed by production-grade CLIs:

- Recursive command trees
- Global flags and command-local flags
- Positional argument schema
- Multi-source binding with `env / config / CLI / default`
- Shell completion
- Machine-readable `spec`
- Markdown and man page generation
- Hooks, middleware, and observers
- Unified CLI errors and exit codes

This document is organized from quick start to platform-style integration.

## Table of Contents

1. [Installation](#installation)
2. [Quick Start](#quick-start)
3. [Two Usage Modes](#two-usage-modes)
4. [Command Model](#command-model)
5. [Flag Model](#flag-model)
6. [Parsing Rules](#parsing-rules)
7. [Help and Built-in Commands](#help-and-built-in-commands)
8. [Config, Environment Variables, and Precedence](#config-environment-variables-and-precedence)
9. [Positional Arguments](#positional-arguments)
10. [Completion](#completion)
11. [Machine-readable Spec](#machine-readable-spec)
12. [Docs Generation](#docs-generation)
13. [Lifecycle Hooks](#lifecycle-hooks)
14. [Middleware](#middleware)
15. [Observers and Telemetry](#observers-and-telemetry)
16. [Unified Errors and Exit Codes](#unified-errors-and-exit-codes)
17. [Custom Extension Metadata](#custom-extension-metadata)
18. [Common Patterns](#common-patterns)
19. [API Quick Reference](#api-quick-reference)

## Installation

Import the package using your actual module path. In this repository, the package path is:

```go
import "pkg.gostartkit.com/cmd"
```

## Quick Start

This minimal example includes:

- A global flag `--verbose`
- A `version` command
- A `hello` command
- Command-local flags, positional arguments, and env binding

```go
package main

import (
	"context"
	"fmt"

	"pkg.gostartkit.com/cmd"
)

var (
	verbose bool
	name    string
	version = "v1.0.0"
)

func main() {
	cmd.SetFlags(func(f *cmd.FlagSet) {
		f.BoolVar(&verbose, "verbose", false, "enable verbose output", "v")
		f.SetCategory("verbose", "Global")
	})

	cmd.AddCommands(
		&cmd.Command{
			Name:      "version",
			UsageLine: "app version",
			Short:     "print version",
			Run: func(ctx context.Context, c *cmd.Command, args []string) error {
				fmt.Println(version)
				return nil
			},
		},
		&cmd.Command{
			Name:      "hello",
			UsageLine: "app hello [flags] <target>",
			Short:     "print greeting",
			Examples: []string{
				"app hello team --name sam",
				"APP_NAME=sam app hello user",
			},
			Positionals: []cmd.PositionalArg{
				{Name: "target", Usage: "greeting target", Required: true, Enum: []string{"team", "user"}},
			},
			SetFlags: func(f *cmd.FlagSet) {
				f.StringVar(&name, "name", "", "name to greet", "n")
				f.BindEnv("name", "APP_NAME")
				f.MarkRequired("name")
				f.SetEnum("name", "sam", "sara", "tom")
			},
			Run: func(ctx context.Context, c *cmd.Command, args []string) error {
				if verbose {
					fmt.Printf("[verbose] target=%s name=%s\n", args[0], name)
				}
				fmt.Printf("hello %s (%s)\n", name, args[0])
				return nil
			},
		},
	)

	cmd.Execute()
}
```

Example invocations:

```bash
app version
app --verbose hello team --name sam
APP_NAME=sara app hello user
app hello team -n tom
```

## Two Usage Modes

The library supports two styles.

### 1. Global default instance: `DefaultApp`

This is the simplest approach for a single binary:

```go
cmd.SetFlags(...)
cmd.AddCommands(...)
cmd.Execute()
```

Global entry points:

- `SetFlags`
- `AddCommands`
- `SetUsageTemplate`
- `Execute`

These helpers are only thin wrappers around the shared `DefaultApp` instance. The real execution model is still `App + Root Command`.

### 2. Explicit instance: `App`

Use this when you need:

- Multiple CLI instances
- Tests
- Embedded execution
- Framework or platform wrappers

```go
app := cmd.NewApp("myapp")
app.SetFlags = func(f *cmd.FlagSet) { ... }
app.Commands = []*cmd.Command{...}

err := app.Run(context.Background(), []string{"hello", "team"})
if err != nil {
	// handle
}
```

If you want to stay on instance-style APIs without mutating fields directly, the app also provides thin helpers:

```go
app := cmd.NewApp("myapp")
app.ConfigureFlags(func(f *cmd.FlagSet) { ... })
app.SetRootCommand(&cmd.Command{ ... })
app.AddCommands(...)

err := app.Execute([]string{"hello", "team"})
```

## Command Model

The core types are `App` and `Command`.

### App

`App` represents the entire CLI application. Common fields:

- `Name`: application name
- `Short`: short description
- `Long`: long description
- `Root`: optional root command
- `Commands`: top-level commands
- `SetFlags`: register global flags
- `BeforeRun / AfterRun / OnError`: lifecycle hooks
- `Middlewares`: middleware chain
- `Observers`: event observers
- `Extensions`: custom metadata

### Root Command

`App` can also own a real root command through `App.Root`.

- `App.SetFlags` remains the app-level global flag entry point.
- `App.Commands` remains compatible and is treated as root subcommands.
- `App.Root.SetFlags` is merged into the root/global flag set and is also visible to subcommands.
- If the root command has `Run`, invoking only the binary runs the root command.
- If the root command has no `Run` but has subcommands, invoking only the binary shows usage.

```go
app := cmd.NewApp("myapp")

var (
	verbose bool
	profile string
)

app.SetFlags = func(f *cmd.FlagSet) {
	f.BoolVar(&verbose, "verbose", false, "enable verbose output", "v")
}

app.Root = &cmd.Command{
	UsageLine: "myapp [flags] [target]",
	Short:     "root entrypoint",
	Examples:  []string{"myapp team", "myapp version"},
	Positionals: []cmd.PositionalArg{
		{Name: "target", Usage: "target name"},
	},
	SetFlags: func(f *cmd.FlagSet) {
		f.StringVar(&profile, "profile", "", "profile name", "p")
	},
	Run: func(ctx context.Context, c *cmd.Command, args []string) error {
		fmt.Printf("root args=%v verbose=%v profile=%s\n", args, verbose, profile)
		return nil
	},
	SubCommands: []*cmd.Command{
		{
			Name:      "version",
			UsageLine: "myapp version",
			Short:     "print version",
			Run: func(ctx context.Context, c *cmd.Command, args []string) error {
				fmt.Println("v1.0.0")
				return nil
			},
		},
	},
}
```

### Command

`Command` represents a node in the command tree. Common fields:

- `Name`
- `Aliases`
- `UsageLine`
- `Short`
- `Long`
- `Examples`
- `Positionals`
- `SetFlags`
- `Run`
- `SubCommands`
- `Deprecated`
- `Hidden`
- `BeforeRun / AfterRun / OnError`
- `Middlewares`
- `Observers`
- `Extensions`

### Defining Subcommands

```go
cmdAdmin := &cmd.Command{
	Name:      "admin",
	UsageLine: "app admin",
	Short:     "admin operations",
	SubCommands: []*cmd.Command{
		{
			Name:      "users",
			UsageLine: "app admin users",
			Short:     "manage users",
			Run: func(ctx context.Context, c *cmd.Command, args []string) error {
				return nil
			},
		},
	},
}
```

## Flag Model

Flags are managed through `FlagSet`. The library supports:

- Global flags
- Command-local flags
- Positional arguments

### Defining Flags

```go
var (
	force  bool
	count  int
	format string
)

f.BoolVar(&force, "force", false, "force operation", "f")
f.IntVar(&count, "count", 1, "retry count", "c")
f.StringVar(&format, "format", "text", "output format", "")
```

Supported value types include:

- `BoolVar`
- `IntVar`
- `Int64Var`
- `UintVar`
- `Uint64Var`
- `StringVar`
- `Float64Var`
- `DurationVar`
- `TextVar`
- `Func`
- `BoolFunc`

There are also global versions of the same helpers operating on the default `CommandLine`.

### Flag Metadata

After defining a flag, you can attach metadata:

```go
f.StringVar(&format, "format", "text", "output format", "")
f.BindEnv("format", "APP_FORMAT")
f.BindConfig("format", "output.format")
f.SetEnum("format", "json", "yaml", "text")
f.MarkRequired("format")
f.MarkHidden("format")
f.MarkDeprecated("format", "use --output instead")
f.SetCategory("format", "Output")
f.SetExample("format", "json")
```

These metadata fields affect:

- Parsing and validation
- Help output
- Completion
- `spec`
- `docs`

## Parsing Rules

The parsing behavior is one of the main features of the library.

### 1. Global flags are allowed before the command

```bash
app --verbose version
app --config app.json hello
```

### 2. Command flags and positional arguments may be interspersed

```bash
app hello team -n sam
app hello team --name sam
app hello team extra --name sam
```

In other words, command flags do not need to appear before all positional arguments.

### 3. `--` stops flag parsing

```bash
app hello -- --name-not-a-flag
```

### 4. `help` routes according to command context

```bash
app help hello
app --verbose help hello
app hello --help
```

### 5. Typos get suggestions

The library suggests close matches for unknown commands and flags:

- `statu` -> `status`
- `--verboes` -> `--verbose`

## Help and Built-in Commands

Built-ins:

- `help`
- `completion`
- `spec`
- `docs`

If you define a user command with the same name, the user command wins and the built-in is skipped.

### Custom Usage Template

If you use the default global instance, you can replace the default usage template:

```go
cmd.SetUsageTemplate(`
{{.Name}} - {{.Short}}

Usage:
  {{.Name}} [flags] <command>
`)
```

## Config, Environment Variables, and Precedence

### Enable JSON Config Support

```go
app := cmd.NewApp("app")
app.EnableConfigSupport()
```

Once enabled, the library injects a built-in global flag:

```bash
app --config app.json hello
```

The default loader expects JSON:

```json
{
  "name": "from-config",
  "output": {
    "format": "json"
  }
}
```

### Bind an env var

```go
f.StringVar(&name, "name", "", "target name", "n")
f.BindEnv("name", "APP_NAME", "LEGACY_NAME")
```

### Bind a config key

```go
f.StringVar(&format, "format", "", "output format", "")
f.BindConfig("format", "output.format")
```

### Precedence

The precedence is fixed:

`CLI flag > env > config > default`

Examples:

```bash
app --config app.json hello
APP_NAME=sam app --config app.json hello
app --config app.json hello --name cli
```

### Custom Config Entry Points

You can also override:

- `ConfigLoader`
- `ConfigFlag`

For example, to plug in your own config loader.

## Positional Arguments

Positional arguments are described through `Command.Positionals`.

### Basic Usage

```go
cmdDeploy := &cmd.Command{
	Name:      "deploy",
	UsageLine: "app deploy <env> [service]",
	Positionals: []cmd.PositionalArg{
		{Name: "env", Usage: "target environment", Required: true, Enum: []string{"dev", "staging", "prod"}},
		{Name: "service", Usage: "service name"},
	},
	Run: func(ctx context.Context, c *cmd.Command, args []string) error {
		env := args[0]
		service := ""
		if len(args) > 1 {
			service = args[1]
		}
		_ = env
		_ = service
		return nil
	},
}
```

### Variadic positional arguments

```go
Positionals: []cmd.PositionalArg{
	{Name: "files", Variadic: true, Usage: "input files"},
}
```

### Completion

Positional arguments also support:

- `Enum`
- `Completion`
- `Extensions`

```go
Positionals: []cmd.PositionalArg{
	{
		Name: "service",
		Completion: func(ctx cmd.CompletionContext) []string {
			return []string{"api", "worker", "web"}
		},
	},
}
```

### Validation

The library automatically handles:

- Missing required positional arguments
- Extra arguments for non-variadic commands
- Enum validation

## Completion

### Generate shell scripts

```bash
app completion bash > /etc/bash_completion.d/app
app completion zsh > "${fpath[1]}/_app"
app completion fish > ~/.config/fish/completions/app.fish
app completion powershell > app.ps1
```

### What completion supports

- Command names
- Command aliases
- Global flags
- Command-local flags
- Flag enum values
- Flag dynamic completion
- Positional enum values
- Positional dynamic completion
- Built-in commands and their arguments

Example:

```go
f.StringVar(&format, "format", "", "output format", "f")
f.SetEnum("format", "json", "yaml", "text")

f.StringVar(&name, "name", "", "target name", "n")
f.SetCompletion("name", func(ctx cmd.CompletionContext) []string {
	return []string{"sam", "sara", "tom"}
})
```

Built-ins also have completion:

- `app completion <shell>`
- `app spec json`
- `app docs markdown`
- `app docs man`

## Machine-readable Spec

### Output

```bash
app spec
app spec json
```

### What `spec` includes

`spec` is a versioned contract for the current command tree. It is suitable for:

- Static site generation
- IDE integration
- Console UIs
- Agent and AI pipelines
- Automated tests

Current output includes:

- `schema_version`
- `builtins`
- `capabilities`
- `config`
- App and command hooks
- Middleware and observer markers
- Global flags
- The command tree
- Positionals
- Flags
- `extensions`

### Important fields in `FlagSpec` and `PositionalSpec`

- `enum`
- `required`
- `deprecated`
- `supports_completion`
- `source_order`
- `extensions`

### Example

```bash
app spec json > spec.json
```

Example fields:

```json
{
  "schema_version": "v2",
  "name": "app",
  "builtins": ["help", "completion", "spec", "docs"],
  "capabilities": {
    "docs_export": true,
    "middleware": true,
    "observers": true
  }
}
```

## Docs Generation

`docs` is generated from `Spec()`, so it shares the same command contract used by `spec`, completion, and help output.

### Single-page output

```bash
app docs markdown
app docs man
```

### Multi-file export

```bash
app docs markdown ./docs
app docs man ./manpages
```

### Export layout

#### Markdown bundle

- `README.md`
- `commands/<command>.md`
- `commands/<command>/<subcommand>.md`

#### Man bundle

- `<app>.1`
- `<app>-<command>.1`
- `<app>-<command>-<subcommand>.1`

### Markdown frontmatter

Markdown docs include frontmatter automatically, which is useful for:

- Hugo, Docusaurus, Astro, MkDocs, and similar site generators
- Search indexers
- Content pipelines

Frontmatter includes:

- `kind`
- `title`
- `summary`
- `command_name`
- `command_path`
- `extensions`

## Lifecycle Hooks

Hooks are best for execution lifecycle behavior, not wrapping-style cross-cutting control.

### App-level hooks

```go
app.BeforeRun = func(ctx cmd.HookContext) error {
	return nil
}

app.AfterRun = func(ctx cmd.HookContext) {
	if ctx.Err != nil {
		log.Printf("command failed: %v", ctx.Err)
	}
}

app.OnError = func(ctx cmd.HookContext) {
	var cliErr *cmd.CLIError
	if errors.As(ctx.Err, &cliErr) {
		log.Printf("kind=%s exit=%d command=%s", cliErr.Kind, cliErr.ExitCode, cliErr.Command)
	}
}
```

### Command-level hooks

```go
cmdDeploy := &cmd.Command{
	Name: "deploy",
	BeforeRun: func(ctx cmd.HookContext) error {
		return nil
	},
	AfterRun: func(ctx cmd.HookContext) {},
	OnError:  func(ctx cmd.HookContext) {},
	Run:      runDeploy,
}
```

### Invocation order

On success:

1. App `BeforeRun`
2. Command `BeforeRun`
3. `Run`
4. Command `AfterRun`
5. App `AfterRun`

On failure:

1. App `BeforeRun`
2. Command `BeforeRun`
3. `Run` or a hook returns an error
4. Command `OnError`
5. App `OnError`
6. Command `AfterRun`
7. App `AfterRun`

## Middleware

Middleware is for cross-cutting behavior such as:

- Authentication
- Tracing
- Rate limiting
- Auditing
- Unified logging

### App-level middleware

```go
app.Use(func(ctx cmd.MiddlewareContext, next cmd.NextFunc) error {
	start := time.Now()
	err := next(ctx.Context)
	log.Printf("command=%s duration=%s err=%v", ctx.Command.Name, time.Since(start), err)
	return err
})
```

### Command-level middleware

```go
cmdDeploy := &cmd.Command{
	Name: "deploy",
	Middlewares: []cmd.Middleware{
		func(ctx cmd.MiddlewareContext, next cmd.NextFunc) error {
			if len(ctx.Args) == 0 {
				return errors.New("missing target")
			}
			return next(ctx.Context)
		},
	},
	Run: runDeploy,
}
```

### Wrapping order

Execution order is:

`app middleware -> command middleware -> Command.Run`

## Observers and Telemetry

Observers provide a stable event stream for:

- Metrics
- Tracing adapters
- Event logs
- Analytics

### Register an observer

```go
app.AddObserver(cmd.ObserverFunc(func(event cmd.Event) {
	log.Printf(
		"type=%s command=%s exit=%d duration=%s",
		event.Type,
		event.Command.Name,
		event.ExitCode,
		event.Duration,
	)
}))
```

Commands can also register their own `Observers`.

### Current event types

- `command_started`
- `command_finished`
- `command_failed`

### Event fields

`Event` includes:

- `Type`
- `App`
- `Command`
- `Args`
- `Err`
- `StartTime`
- `EndTime`
- `Duration`
- `ExitCode`

## Unified Errors and Exit Codes

Library-generated normalized errors are returned as `*CLIError`.

### Error kinds

Current `Kind` values:

- `invalid_arguments`
- `not_found`
- `canceled`
- `internal`
- `runtime`

### Exit codes

- Invalid arguments: `2`
- Unknown command: `2`
- `context.Canceled`: `130`
- `context.DeadlineExceeded`: `124`
- Runtime failure: `1`

### Example

```go
err := app.Run(ctx, os.Args[1:])
if err != nil {
	var cliErr *cmd.CLIError
	if errors.As(err, &cliErr) {
		fmt.Println(cliErr.Kind, cliErr.ExitCode, cliErr.Command)
	}
}
```

`Execute()` automatically exits using `ExitStatus()` on the default instance.

## Custom Extension Metadata

If you need to attach custom metadata to the command tree, for example:

- Site categorization
- Console UI hints
- Internal ownership
- Feature flags
- OpenAPI or agent-specific extension fields

use `extensions`.

### App-level

```go
app.SetExtension("x-site-section", "cli")
```

### Command-level

```go
cmdDeploy.SetExtension("x-owner", "platform")
```

### Positional-level

```go
cmdDeploy.Positionals[0].SetExtension("x-label", "Environment")
```

### Flag-level

```go
cmdDeploy.SetFlags = func(f *cmd.FlagSet) {
	f.StringVar(&format, "format", "", "output format", "")
	_ = f.SetExtension("format", "x-ui-control", "select")
}
```

These metadata fields are exported into:

- `spec`
- Markdown frontmatter

## Common Patterns

### 1. Global config plus command-local flags

```go
app := cmd.NewApp("app")
app.EnableConfigSupport()

app.SetFlags = func(f *cmd.FlagSet) {
	f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
}

app.Commands = []*cmd.Command{
	{
		Name: "sync",
		SetFlags: func(f *cmd.FlagSet) {
			f.StringVar(&endpoint, "endpoint", "", "api endpoint", "")
			f.BindEnv("endpoint", "APP_ENDPOINT")
			f.BindConfig("endpoint", "api.endpoint")
			f.MarkRequired("endpoint")
		},
		Run: runSync,
	},
}
```

### 2. Use enums for both validation and completion

```go
f.StringVar(&env, "env", "", "target environment", "")
f.SetEnum("env", "dev", "staging", "prod")
```

### 3. Use observers for metrics

```go
app.AddObserver(cmd.ObserverFunc(func(event cmd.Event) {
	switch event.Type {
	case cmd.EventCommandFinished:
		metrics.RecordSuccess(event.Command.Name, event.Duration)
	case cmd.EventCommandFailed:
		metrics.RecordFailure(event.Command.Name, event.Duration)
	}
}))
```

### 4. Use `docs` and `spec` to drive docs sites and consoles

```bash
app spec json > site/spec.json
app docs markdown ./site/docs
```

## API Quick Reference

### Application and commands

- `NewApp(name string) *App`
- `(*App).Run(ctx, args)`
- `(*App).EnableConfigSupport()`
- `(*App).Use(...)`
- `(*App).AddObserver(...)`
- `(*App).SetExtension(key, value)`
- `(*App).Spec()`

### Default instance

- `SetFlags(...)`
- `AddCommands(...)`
- `SetUsageTemplate(...)`
- `Execute()`

### FlagSet metadata helpers

- `BindEnv`
- `BindConfig`
- `SetEnum`
- `SetCompletion`
- `MarkRequired`
- `MarkHidden`
- `MarkDeprecated`
- `SetCategory`
- `SetExample`
- `SetExtension`

### Important types

- `App`
- `Command`
- `FlagSet`
- `Flag`
- `PositionalArg`
- `CompletionContext`
- `HookContext`
- `MiddlewareContext`
- `Event`
- `CLIError`
- `AppSpec`

## Summary

If you only need a small CLI, these are enough:

- `SetFlags`
- `AddCommands`
- `Execute`

If you want a CLI that can grow into a platform, organize around this model:

- `Command / Flag / Positional` as the single command model
- `env / config / CLI` as the single source-of-truth for configuration resolution
- `hooks / middleware / observer` as the runtime extension layer
- `spec / docs` as the external contract

That is the direction this library is best suited for today.
