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
  {{.Name | printf "%-15s"}} {{.Short}}{{end}}{{end}}

options:

  -v --verbose   make the operation more talkative

Use "{{.Name}} [command] --help" for more information about a command.
`

	_commands   = Commands{}
	_exitMu     sync.Mutex
	_exitStatus = 0
	_setFlags   func(f *flag.FlagSet)
)

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

// findCommand recursively finds a command or subcommand
func findCommand(cmds Commands, args []string) (*Command, []string, error) {
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("no command provided: %w", ErrNotFound)
	}

	cmd := cmds.Search(args[0])

	if cmd == nil {
		return nil, nil, fmt.Errorf("unknown command %q: %w", args[0], ErrNotFound)
	}

	if len(args) > 1 && len(cmd.SubCommands) > 0 {
		subCmd, remainingArgs, err := findCommand(cmd.SubCommands, args[1:])
		if err == nil {
			return subCmd, remainingArgs, nil
		}
	}

	return cmd, args[1:], nil
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

	addFlags(&cmd.Flag)

	if cmd.SetFlags != nil {
		cmd.SetFlags(&cmd.Flag)
	}

	cmd.Flag.Usage = func() { cmd.Usage() }

	if err := cmd.Flag.Parse(remainingArgs); err != nil {
		fatalf("cmd(%s): %v \n", name, err)
	}

	if err := cmd.Run(cmd, cmd.Flag.Args()); err != nil {
		fatalf("cmd(%s): %v\n", name, err)
	}

	exit()
}

// Command struct
type Command struct {
	Name        string
	Aliases     []string
	UsageLine   string
	Short       string
	Long        string
	Run         func(cmd *Command, args []string) error
	SetFlags    func(f *flag.FlagSet)
	Flag        flag.FlagSet
	SubCommands Commands
}

// Usage u
func (c *Command) Usage() {
	help([]string{c.Name})
	os.Exit(2)
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

	if len(args) > 1 {
		fatalf("Usage: help <command>\n\nToo many arguments.\n")
	}

	name := args[0]

	cmd, _, err := findCommand(_commands, args)

	if err != nil {
		fatalf("help(%s): %v \n", name, err)
	}

	if cmd.Runnable() {
		fmt.Fprintf(os.Stdout, "usage: %s\n\n", cmd.UsageLine)
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
