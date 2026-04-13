package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"text/template"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNotFound = errors.New("not found")

	defaultUsageTemplate = `
{{.Name}} - {{.Short}}

Usage:

  {{.Name}} [flags] <command> [subcommand] [args]

Available Commands:
{{range .Commands}}{{if .Runnable}}
  {{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}

options:

  -v --verbose   make the operation more talkative

Use "{{.Name}} [command] --help" for more information about a command.
`

	DefaultApp = NewApp(filepath.Base(os.Args[0]))
)

// App is the main application container
type App struct {
	Name          string
	Short         string
	Long          string
	Commands      Commands
	UsageTemplate string
	Out           io.Writer
	Err           io.Writer
	SetFlags      func(f *FlagSet)

	mu         sync.Mutex
	exitStatus int
}

// NewApp creates a new App instance
func NewApp(name string) *App {
	return &App{
		Name:          name,
		Short:         "Command-line tool",
		UsageTemplate: defaultUsageTemplate,
		Out:           os.Stdout,
		Err:           os.Stderr,
	}
}

// Command struct
type Command struct {
	Name        string
	Aliases     []string
	UsageLine   string
	Short       string
	Long        string
	Run         func(ctx context.Context, cmd *Command, args []string) error
	SetFlags    func(f *FlagSet)
	SubCommands Commands

	alias string
	flag  *FlagSet
	app   *App
}

// GetAlias get alias
func (c *Command) GetAlias() string {
	return c.alias
}

// Usage u
func (c *Command) Usage() {
	out := c.app.Out
	if out == nil {
		out = os.Stdout
	}

	fmt.Fprintf(out, "\nUsage: %s\n\n", c.UsageLine)

	if c.Aliases != nil {
		fmt.Fprintf(out, "  Aliases: %s\n\n", strings.Join(c.Aliases, ", "))
	}

	if c.Long != "" {
		runTemplate(out, c.Long, c)
		fmt.Fprintf(out, "\n\n")
	}

	// Display subcommands if any
	if len(c.SubCommands) > 0 {
		fmt.Fprintf(out, "Available Subcommands:\n")

		maxLen := 0

		for _, sub := range c.SubCommands {
			if sub.Runnable() {
				nameLen := len(sub.Name)
				if nameLen > maxLen {
					maxLen = nameLen
				}
			}
		}

		for _, sub := range c.SubCommands {
			if sub.Runnable() {
				fmt.Fprintf(out, "  %-*s %s\n", maxLen+2, sub.Name, sub.Short)
			}
		}

		fmt.Fprintf(out, "\n")
	}

	if c.flag != nil {
		// Display flags
		fmt.Fprintf(out, "Flags:\n")

		maxLen := 0

		c.flag.VisitAll(func(f *Flag) {
			nameLen := len(f.Name)
			if nameLen > maxLen {
				maxLen = nameLen
			}
		})

		maxLen += 2

		c.flag.VisitAll(func(f *Flag) {

			if len(f.Shorthand) > 0 {
				fmt.Fprintf(out, "  -%s --%-*s %s\n", f.Shorthand, maxLen, f.Name, f.Usage)
			} else {
				fmt.Fprintf(out, "     --%-*s %s\n", maxLen, f.Name, f.Usage)
			}
		})

		fmt.Fprintf(out, "\n")
	}

}

// Runnable bool
func (c *Command) Runnable() bool {
	return c.Run != nil
}

type Commands []*Command

// Search use binary search to find and return the smallest index *Command
func (c *Commands) Search(name string) *Command {

	for _, cmd := range *c {
		if cmd.Name == name {
			return cmd
		}
	}

	for _, cmd := range *c {
		if slices.Contains(cmd.Aliases, name) {
			return cmd
		}
	}

	return nil
}

// SetUsageTemplate set value to usageTemplate
func SetUsageTemplate(usageTemplate string) {
	DefaultApp.UsageTemplate = usageTemplate
}

// SetFlags set flags to all commands
func SetFlags(f func(f *FlagSet)) {
	DefaultApp.SetFlags = f
}

// AddCommands Add Command.
func AddCommands(cmds ...*Command) {
	DefaultApp.Commands = append(DefaultApp.Commands, cmds...)
}

// Execute func
func Execute() {
	if err := DefaultApp.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(DefaultApp.Err, "Error: %v\n", err)
		os.Exit(DefaultApp.exitStatus)
	}
	os.Exit(DefaultApp.exitStatus)
}

// Run executes the application
func (a *App) Run(ctx context.Context, args []string) error {
	log.SetFlags(0)

	// Preliminary check for help
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			if arg == "help" && len(args) > 1 {
				return a.help(args[1:])
			}
			a.Usage()
			return nil
		}
	}

	if len(args) < 1 {
		a.Usage()
		a.setExitStatus(2)
		return nil
	}

	name := args[0]
	cmd, remainingArgs, err := findCommand(a.Commands, args)

	if err != nil {
		suggestions := suggestCommand(name, a.Commands)
		if len(suggestions) > 0 {
			return fmt.Errorf("%w, unknown command %q. Did you mean %s?", ErrNotFound, name, strings.Join(suggestions, " or "))
		}
		return fmt.Errorf("%w, unknown command %q", ErrNotFound, name)
	}

	cmd.app = a

	if cmd.flag == nil {
		cmd.flag = NewFlagSet(cmd.Name, ContinueOnError)
	}
	cmd.flag.SetOutput(a.Err)

	if a.SetFlags != nil {
		a.SetFlags(cmd.flag)
	}

	if cmd.SetFlags != nil {
		cmd.SetFlags(cmd.flag)
	}

	cmd.flag.Usage = func() {
		cmd.Usage()
	}

	if err := cmd.flag.Parse(remainingArgs); err != nil {
		a.setExitStatus(2)
		return err
	}

	if err := cmd.Run(ctx, cmd, cmd.flag.Args()); err != nil {
		a.setExitStatus(1)
		return err
	}

	return nil
}

func (a *App) setExitStatus(n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.exitStatus < n {
		a.exitStatus = n
	}
}

func (a *App) Usage() {
	data := struct {
		Name     string
		Short    string
		Commands Commands
	}{
		Name:     a.Name,
		Short:    a.Short,
		Commands: a.Commands,
	}
	bw := bufio.NewWriter(a.Err)
	runTemplate(bw, a.UsageTemplate, data)
	bw.Flush()
}

func (a *App) help(args []string) error {
	if len(args) == 0 {
		a.Usage()
		return nil
	}

	cmd, _, err := findCommand(a.Commands, args)
	if err != nil {
		return err
	}
	cmd.app = a
	cmd.Usage()
	return nil
}

// findCommand recursively finds a command or subcommand
func findCommand(cmds Commands, args []string) (*Command, []string, error) {

	if len(args) == 0 {
		return nil, nil, fmt.Errorf("%w, no command provided", ErrNotFound)
	}

	name := args[0]

	cmd := cmds.Search(name)

	if cmd == nil {
		return nil, nil, fmt.Errorf("%w, unknown command %q", ErrNotFound, name)
	}

	cmd.alias = name

	if len(args) > 1 && len(cmd.SubCommands) > 0 {

		subCmd, remainingArgs, err := findCommand(cmd.SubCommands, args[1:])

		if err == nil {
			return subCmd, remainingArgs, nil
		}
	}

	return cmd, args[1:], nil
}

type errWriter struct {
	w   io.Writer
	err error
}

func (w *errWriter) Write(b []byte) (int, error) {
	n, err := w.w.Write(b)
	if err != nil {
		w.err = err
	}
	return n, err
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToTitle(r)) + s[n:]
}

func runTemplate(w io.Writer, text string, data interface{}) {
	t := template.New("top")
	t.Funcs(template.FuncMap{
		"trim":       strings.TrimSpace,
		"capitalize": capitalize,
	})
	template.Must(t.Parse(text))
	ew := &errWriter{w: w}
	err := t.Execute(ew, data)
	if ew.err != nil {
		if strings.Contains(ew.err.Error(), "pipe") {
			os.Exit(1)
		}
	}
	if err != nil {
		panic(err)
	}
}
