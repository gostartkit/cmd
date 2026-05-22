/*
Package cmd provides a modern command-line library for Go.

The package is centered around App and Command. It supports recursive
subcommands, global and command-local flags, positional argument schemas,
environment and config binding, shell completion, machine-readable specs,
generated documentation, lifecycle hooks, middleware, observer events,
normalized CLI errors, and a built-in REPL that reuses the same command tree.

The simplest usage relies on the default application instance:

	cmd.SetFlags(func(f *cmd.FlagSet) {
		f.BoolVar(&verbose, "verbose", false, "enable verbose output", "v")
	})

	cmd.AddCommands(&cmd.Command{
		Name:      "version",
		UsageLine: "app version",
		Short:     "print version",
		Run: func(ctx context.Context, c *cmd.Command, args []string) error {
			fmt.Println(version)
			return nil
		},
	})

	cmd.Execute()

For larger applications, create an explicit App with NewApp, attach commands,
configure flags and config support, and call Run directly.

When an application needs both CLI and interactive usage, define the command
tree once and enable the built-in REPL on the same App. CLI execution, REPL
execution, completion, and inline REPL hints all share the same parser and
command model. Terminal REPL sessions support editable input, history
navigation, completion, dynamic prompts, and optional history load/append
hooks.

Runtime helpers are also available so applications can choose explicit CLI or
REPL entrypoints, or let the library automatically select an appropriate
runtime for the current invocation.

Internally, both CLI args and REPL lines flow through the same Registry,
Resolver, and Dispatcher pipeline. That shared pipeline keeps help,
completion, spec export, docs generation, and command execution aligned on
the same effective command tree while still isolating runtime flag state for
each invocation.

See README.md for the full usage guide, including config precedence, completion,
REPL configuration, runtime helpers, spec export, documentation generation,
hooks, middleware, observers, and custom extensions.
*/
package cmd
