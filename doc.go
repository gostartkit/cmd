/*
Package cmd provides a modern command-line library for Go.

The package is centered around App and Command. It supports recursive
subcommands, global and command-local flags, positional argument schemas,
environment and config binding, shell completion, machine-readable specs,
generated documentation, lifecycle hooks, middleware, observer events, and
normalized CLI errors.

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

See README.md for the full usage guide, including config precedence, completion,
spec export, documentation generation, hooks, middleware, observers, and custom
extensions.
*/
package cmd
