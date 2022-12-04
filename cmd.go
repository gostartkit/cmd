package cmd

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"text/template"
	"unicode"
	"unicode/utf8"
)

var (
	_usageTemplate = `[webgo] is a web service base on web.go
Usage:
	[webgo] command [arguments]

The commands are:
{{range .}}{{if .Runnable}}
	{{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}

Use "[webgo] help [command]" for more information about a command.
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

// searchCommand search command by name.
func searchCommand(name string) (*Command, error) {
	if len(_commands) == 0 {
		return nil, fmt.Errorf("no commands")
	}

	cmd := _commands.Search(name)

	if cmd == nil {
		return nil, fmt.Errorf("unknown command %q", name)
	}

	return cmd, nil
}

// Execute func
func Execute(force bool) {
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
	cmd, err := searchCommand(name)

	if err != nil {
		fatalf("cmd(%s): %v \n", name, err)
	}

	addFlags(&cmd.Flag)
	cmd.Flag.Usage = func() { cmd.Usage() }
	cmd.Flag.Parse(args[1:])

	if err := cmd.Run(cmd, cmd.Flag.Args(), force); err != nil {
		logf("cmd(%s): %v\n", name, err)
	}

	exit()
}

// Command struct
type Command struct {
	Run       func(cmd *Command, args []string, force bool) error
	Flag      flag.FlagSet
	UsageLine string
	Short     string
	Long      string
}

// Name string
func (c *Command) Name() string {
	name := c.UsageLine
	i := strings.IndexRune(name, ' ')
	if i >= 0 {
		name = name[:i]
	}
	return name
}

// Usage u
func (c *Command) Usage() {
	help([]string{c.Name()})
	os.Exit(2)
}

// Runnable bool
func (c *Command) Runnable() bool {
	return c.Run != nil
}

type Commands []*Command

// Search use binary search to find and return the smallest index *Command
func (c *Commands) Search(name string) *Command {

	i := sort.Search(len(*c), func(i int) bool { return (*c)[i].Name() >= name })

	if i < len(*c) && (*c)[i].Name() == name {
		return (*c)[i]
	}

	return nil
}

func usage() {
	printUsage(os.Stderr)
	os.Exit(2)
}

func printUsage(w io.Writer) {
	bw := bufio.NewWriter(w)
	runTemplate(bw, _usageTemplate, _commands)
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
		printUsage(os.Stdout)
		return
	}
	if len(args) != 1 {
		fatalf("usage: help command\n\nToo many arguments given.\n")
	}

	name := args[0]

	cmd, err := searchCommand(name)

	if err != nil {
		fatalf("help(%s): %v \n", name, err)
	}

	if cmd.Runnable() {
		fmt.Fprintf(os.Stdout, "usage: %s\n", cmd.UsageLine)
	}

	runTemplate(os.Stdout, cmd.Long, nil)
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
