package cmd

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNotFound = errors.New("not found")

	_usageTemplate = `
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

	_commands   = Commands{}
	_exitMu     sync.Mutex
	_exitStatus = 0
	_setFlags   func(f *flag.FlagSet)
)

// Command struct
type Command struct {
	Name        string
	Aliases     []string
	UsageLine   string
	Short       string
	Long        string
	Run         func(cmd *Command, args []string) error
	SetFlags    func(f *flag.FlagSet)
	SubCommands Commands

	flag *flag.FlagSet
}

// Usage u
func (c *Command) Usage() {

	fmt.Fprintf(os.Stdout, "\nUsage: %s\n\n", c.UsageLine)

	if c.Aliases != nil {
		fmt.Fprintf(os.Stdout, "  Aliases: %s\n\n", strings.Join(c.Aliases, ", "))
	}

	if c.Long != "" {
		runTemplate(os.Stdout, c.Long, c)
		fmt.Fprintf(os.Stdout, "\n\n")
	}

	// Display subcommands if any
	if len(c.SubCommands) > 0 {
		fmt.Fprintf(os.Stdout, "Available Subcommands:\n")

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
				fmt.Fprintf(os.Stdout, "  %-*s %s\n", maxLen+2, sub.Name, sub.Short)
			}
		}

		fmt.Fprintf(os.Stdout, "\n")
	}

	if c.flag != nil {
		// Display flags
		fmt.Fprintf(os.Stdout, "Flags:\n")

		maxLen := 0

		c.flag.VisitAll(func(f *flag.Flag) {
			nameLen := len(f.Name)
			if nameLen > maxLen {
				maxLen = nameLen
			}
		})

		c.flag.VisitAll(func(f *flag.Flag) {

			if len(f.Name) > 1 {
				fmt.Fprintf(os.Stdout, "  --%-*s %s\n", maxLen+2, f.Name, f.Usage)
			} else {
				fmt.Fprintf(os.Stdout, "  -%-*s %s\n", maxLen+3, f.Name, f.Usage)
			}
		})

		fmt.Fprintf(os.Stdout, "\n")
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
		if cmd.Name == name || contains(cmd.Aliases, name) {
			return cmd
		}
	}

	return nil
}

// SetUsageTemplate set value to usageTemplate
func SetUsageTemplate(usageTemplate string) {
	_usageTemplate = usageTemplate
}

// SetFlags set flags to all commands
func SetFlags(f func(f *flag.FlagSet)) {
	_setFlags = f
}

// AddCommands Add Command.
func AddCommands(cmds ...*Command) {
	_commands = append(_commands, cmds...)
}

// Execute func
func Execute() {

	flag.Usage = usage
	flag.Parse() // catch -h argument
	log.SetFlags(0)

	args := flag.Args()

	if len(args) < 1 {
		usage()
	}

	if args[0] == "help" {
		help(args[1:])
		return
	}

	name := args[0]
	cmd, remainingArgs, err := findCommand(_commands, args)

	if err != nil {
		fatalf("cmd(%s): %v \n", name, err)
	}

	if cmd.flag == nil {
		cmd.flag = flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	}

	addFlags(cmd.flag)

	if cmd.SetFlags != nil {
		cmd.SetFlags(cmd.flag)
	}

	cmd.flag.Usage = func() {
		cmd.Usage()
	}

	if err := cmd.flag.Parse(remainingArgs); err != nil {
		fatalf("cmd(%s): %v \n", name, err)
	}

	if err := cmd.Run(cmd, cmd.flag.Args()); err != nil {
		fatalf("cmd(%s): %v\n", name, err)
	}

	exit()
}

// findCommand recursively finds a command or subcommand
func findCommand(cmds Commands, args []string) (*Command, []string, error) {
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("%w, no command provided", ErrNotFound)
	}

	cmd := cmds.Search(args[0])

	if cmd == nil {
		return nil, nil, fmt.Errorf("%w, unknown command %q", ErrNotFound, args[0])
	}

	if len(args) > 1 && len(cmd.SubCommands) > 0 {

		subCmd, remainingArgs, err := findCommand(cmd.SubCommands, args[1:])

		if err == nil {
			return subCmd, remainingArgs, nil
		}
	}

	return cmd, args[1:], nil
}

func usage() {
	progName := filepath.Base(os.Args[0])
	data := struct {
		Name     string
		Short    string
		Commands Commands
	}{
		Name:     progName,
		Short:    "Command-line tool",
		Commands: _commands,
	}
	printUsage(os.Stderr, data)
	os.Exit(2)
}

func printUsage(w io.Writer, data interface{}) {
	bw := bufio.NewWriter(w)
	runTemplate(bw, _usageTemplate, data)
	bw.Flush()
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
	if len(os.Args) > 0 {
		text = strings.ReplaceAll(text, "{{.Name}}", filepath.Base(os.Args[0]))
	}
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
		fatalf("writing output: %v", ew.err)
	}
	if err != nil {
		panic(err)
	}
}

func help(args []string) {

	if len(args) == 0 {
		usage()
	}

	name := args[0]

	cmd, _, err := findCommand(_commands, args)

	if err != nil {
		fatalf("help(%s): %v \n", name, err)
	}

	cmd.Usage()
}

func addFlags(f *flag.FlagSet) {
	if _setFlags != nil {
		_setFlags(f)
	}
}

func logf(format string, v ...interface{}) {
	log.Printf(format, v...)
}

func errorf(format string, args ...interface{}) {
	logf(format, args...)
	setExitStatus(1)
}

func fatalf(format string, args ...interface{}) {
	errorf(format, args...)
	exit()
}

func setExitStatus(n int) {
	_exitMu.Lock()
	if _exitStatus < n {
		_exitStatus = n
	}
	_exitMu.Unlock()
}

func exit() {
	os.Exit(_exitStatus)
}

// contains checks if a string exists in a slice
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
