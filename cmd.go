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
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
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
{{range .Commands}}{{if and .Runnable (not .Hidden)}}
  {{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}

Use "{{.Name}} help [command]" for more information about a command.
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
	ConfigEnabled bool
	ConfigLoader  ConfigLoader
	ConfigFlag    ConfigFlagOptions
	BeforeRun     BeforeHook
	AfterRun      AfterHook
	OnError       ErrorHook
	Middlewares   []Middleware
	Observers     []Observer
	Extensions    map[string]any

	mu         sync.Mutex
	exitStatus int
	flag       *FlagSet
	configData map[string]any
}

// NewApp creates a new App instance
func NewApp(name string) *App {
	app := &App{
		Name:          name,
		Short:         "Command-line tool",
		UsageTemplate: defaultUsageTemplate,
		Out:           os.Stdout,
		Err:           os.Stderr,
	}
	app.setDefaultConfigOptions()
	return app
}

func (a *App) EnableConfigSupport() {
	a.ConfigEnabled = true
	a.setDefaultConfigOptions()
	if a.ConfigLoader == nil {
		a.ConfigLoader = LoadJSONConfig
	}
}

func (a *App) setDefaultConfigOptions() {
	if a.ConfigFlag.Name == "" {
		a.ConfigFlag.Name = "config"
	}
	if a.ConfigFlag.Usage == "" {
		a.ConfigFlag.Usage = "path to JSON config file"
	}
	if len(a.ConfigFlag.EnvVars) == 0 && a.Name != "" {
		a.ConfigFlag.EnvVars = []string{defaultConfigEnvVar(a.Name)}
	}
}

func (a *App) configEnabled() bool {
	return a.ConfigEnabled
}

func (a *App) configureFlagSet(flagSet *FlagSet, command *Command) {
	if flagSet == nil {
		return
	}
	if a.configEnabled() {
		a.configureConfigFlag(flagSet)
	}
	if a.SetFlags != nil {
		a.SetFlags(flagSet)
	}
	if command != nil && command.SetFlags != nil {
		command.SetFlags(flagSet)
	}
}

func (a *App) configureConfigFlag(flagSet *FlagSet) {
	if _, exists := flagSet.Lookup(a.ConfigFlag.Name); exists {
		return
	}
	var value string
	flagSet.StringVar(&value, a.ConfigFlag.Name, "", a.ConfigFlag.Usage, a.ConfigFlag.Shorthand)
	flagSet.BindEnv(a.ConfigFlag.Name, a.ConfigFlag.EnvVars...)
	flagSet.SetCategory(a.ConfigFlag.Name, "Global")
	if a.ConfigFlag.Example != "" {
		flagSet.SetExample(a.ConfigFlag.Name, a.ConfigFlag.Example)
	}
}

// Command struct
type Command struct {
	Name        string
	Aliases     []string
	UsageLine   string
	Short       string
	Long        string
	Group       string
	Examples    []string
	Positionals []PositionalArg
	Deprecated  string
	Hidden      bool
	BeforeRun   BeforeHook
	AfterRun    AfterHook
	OnError     ErrorHook
	Middlewares []Middleware
	Observers   []Observer
	Extensions  map[string]any
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

	if len(c.Aliases) > 0 {
		fmt.Fprintf(out, "  Aliases: %s\n\n", strings.Join(c.Aliases, ", "))
	}

	if c.Deprecated != "" {
		fmt.Fprintf(out, "Deprecated: %s\n\n", c.Deprecated)
	}

	if c.Long != "" {
		runTemplate(out, c.Long, c)
		fmt.Fprintf(out, "\n\n")
	}

	// Display subcommands if any
	if len(c.SubCommands) > 0 {
		printCommands(out, "Available Subcommands", c.SubCommands)
	}

	if c.flag != nil {
		printFlagSections(out, "Flags", c.flag)
	}

	printPositionals(out, c.Positionals)

	if len(c.Examples) > 0 {
		fmt.Fprintf(out, "Examples:\n")
		for _, example := range c.Examples {
			fmt.Fprintf(out, "  %s\n", example)
		}
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
		os.Exit(DefaultApp.ExitStatus())
	}
	os.Exit(DefaultApp.ExitStatus())
}

// Run executes the application
func (a *App) Run(ctx context.Context, args []string) error {
	log.SetFlags(0)

	if len(args) < 1 {
		a.Usage()
		a.setExitStatus(2)
		return nil
	}

	if args[0] == "-h" || args[0] == "--help" {
		a.Usage()
		return nil
	}

	if args[0] == "help" {
		return a.help(args[1:])
	}

	a.flag = nil
	a.configData = nil
	a.exitStatus = 0
	if a.SetFlags != nil || a.configEnabled() {
		a.flag = NewFlagSet(a.Name, ContinueOnError)
		a.flag.SetOutput(a.Err)
		a.flag.Usage = a.Usage
		a.configureFlagSet(a.flag, nil)
	}

	parsedAppFlags := []parsedFlagValue{}
	remainingArgs := args
	if a.flag != nil {
		if err := a.flag.ApplyEnv(); err != nil {
			return a.fail(nil, ctx, nil, nil, err, ErrorKindInvalidArguments, 2)
		}
		var err error
		remainingArgs, parsedAppFlags, err = a.parseAppFlags(args)
		if err != nil {
			if errors.Is(err, ErrHelp) {
				return nil
			}
			return a.fail(nil, ctx, nil, nil, err, ErrorKindInvalidArguments, 2)
		}
		if err := a.loadConfigData(); err != nil {
			return a.fail(nil, ctx, nil, nil, err, ErrorKindInvalidArguments, 2)
		}
	}

	if len(remainingArgs) < 1 {
		a.Usage()
		a.setExitStatus(2)
		return nil
	}

	if remainingArgs[0] == "help" {
		return a.help(remainingArgs[1:])
	}

	if handled, err := a.runBuiltinCommand(remainingArgs); handled {
		if err != nil {
			return a.fail(nil, ctx, remainingArgs, nil, err, ErrorKindInvalidArguments, 2)
		}
		return nil
	}

	name := remainingArgs[0]
	cmd, remainingArgs, err := findCommand(a.Commands, remainingArgs)

	if err != nil {
		suggestions := suggestCommand(name, a.Commands)
		if len(suggestions) > 0 {
			return a.fail(nil, ctx, nil, nil, fmt.Errorf("%w, unknown command %q. Did you mean %s?", ErrNotFound, name, strings.Join(suggestions, " or ")), ErrorKindNotFound, 2)
		}
		return a.fail(nil, ctx, nil, nil, fmt.Errorf("%w, unknown command %q", ErrNotFound, name), ErrorKindNotFound, 2)
	}

	cmd.app = a
	startTime := time.Now()

	cmd.flag = NewFlagSet(cmd.Name, ContinueOnError)
	cmd.flag.SetOutput(a.Err)
	a.configureFlagSet(cmd.flag, cmd)

	cmd.flag.Usage = func() {
		cmd.Usage()
	}

	if err := cmd.flag.ApplyConfig(a.configData); err != nil {
		return a.fail(cmd, ctx, remainingArgs, &startTime, err, ErrorKindInvalidArguments, 2)
	}

	if err := cmd.flag.ApplyEnv(); err != nil {
		return a.fail(cmd, ctx, remainingArgs, &startTime, err, ErrorKindInvalidArguments, 2)
	}

	for _, parsedFlag := range parsedAppFlags {
		if err := cmd.flag.Set(parsedFlag.Name, parsedFlag.Value); err != nil {
			return a.fail(cmd, ctx, remainingArgs, &startTime, err, ErrorKindInvalidArguments, 2)
		}
	}

	if err := cmd.flag.Parse(remainingArgs); err != nil {
		if errors.Is(err, ErrHelp) {
			return nil
		}
		return a.fail(cmd, ctx, remainingArgs, &startTime, err, ErrorKindInvalidArguments, 2)
	}

	if err := cmd.flag.Validate(); err != nil {
		return a.fail(cmd, ctx, cmd.flag.Args(), &startTime, err, ErrorKindInvalidArguments, 2)
	}

	if err := cmd.validatePositionals(cmd.flag.Args()); err != nil {
		return a.fail(cmd, ctx, cmd.flag.Args(), &startTime, err, ErrorKindInvalidArguments, 2)
	}

	cmd.flag.WarnDeprecated()
	if cmd.Deprecated != "" {
		fmt.Fprintf(a.Err, "Warning: command %q is deprecated: %s\n", cmd.Name, cmd.Deprecated)
	}

	commandArgs := append([]string(nil), cmd.flag.Args()...)
	a.emitEvent(Event{
		Type:      EventCommandStarted,
		Command:   cmd,
		Args:      commandArgs,
		StartTime: startTime,
	})

	if err := a.runBeforeHooks(ctx, cmd, cmd.flag.Args(), startTime); err != nil {
		return a.fail(cmd, ctx, cmd.flag.Args(), &startTime, err, ErrorKindRuntime, 1)
	}

	middlewareCtx := MiddlewareContext{
		Context:   ctx,
		App:       a,
		Command:   cmd,
		Args:      commandArgs,
		StartTime: startTime,
	}
	runFn := func(runCtx context.Context) error {
		return cmd.Run(runCtx, cmd, commandArgs)
	}
	middlewares := append([]Middleware(nil), a.Middlewares...)
	middlewares = append(middlewares, cmd.Middlewares...)

	if err := chainMiddlewares(middlewareCtx, runFn, middlewares); err != nil {
		return a.fail(cmd, ctx, cmd.flag.Args(), &startTime, err, ErrorKindRuntime, 1)
	}

	a.runAfterHooks(ctx, cmd, cmd.flag.Args(), startTime, nil)
	a.emitEvent(Event{
		Type:      EventCommandFinished,
		Command:   cmd,
		Args:      commandArgs,
		StartTime: startTime,
		EndTime:   time.Now(),
		Duration:  time.Since(startTime),
		ExitCode:  a.ExitStatus(),
	})
	return nil
}

func (a *App) setExitStatus(n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.exitStatus < n {
		a.exitStatus = n
	}
}

func (a *App) ExitStatus() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.exitStatus
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
	if a.flag != nil {
		printFlagSections(bw, "Global Flags", a.flag)
	}
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
	cmd.flag = NewFlagSet(cmd.Name, ContinueOnError)
	cmd.flag.SetOutput(a.Err)
	a.configureFlagSet(cmd.flag, cmd)
	cmd.Usage()
	return nil
}

type parsedFlagValue struct {
	Name  string
	Value string
}

func (a *App) parseAppFlags(args []string) ([]string, []parsedFlagValue, error) {
	if a.flag == nil {
		return args, nil, nil
	}

	parsed := make([]parsedFlagValue, 0)
	for i := 0; i < len(args); {
		arg := args[i]

		switch arg {
		case "-h", "--help":
			a.Usage()
			return nil, nil, ErrHelp
		case "--":
			return args[i+1:], parsed, nil
		}

		if len(arg) < 2 || arg[0] != '-' {
			return args[i:], parsed, nil
		}

		a.flag.args = args[i:]
		beforeArgs := len(a.flag.args)
		beforeActual := len(a.flag.actual)
		seen, err := a.flag.parseOne()
		if err != nil {
			return nil, nil, err
		}
		if !seen {
			return args[i:], parsed, nil
		}

		consumed := beforeArgs - len(a.flag.args)
		if consumed == 0 {
			return args[i:], parsed, nil
		}
		i += consumed

		if len(a.flag.actual) > beforeActual {
			flag := a.flag.actual[len(a.flag.actual)-1]
			parsed = append(parsed, parsedFlagValue{
				Name:  flag.Name,
				Value: flag.Value.String(),
			})
		}
	}

	return nil, parsed, nil
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

func visibleFlags(flagSet *FlagSet) []*Flag {
	if flagSet == nil {
		return nil
	}

	flags := make([]*Flag, 0)
	flagSet.VisitAll(func(flag *Flag) {
		if flag.Hidden {
			return
		}
		flags = append(flags, flag)
	})
	return flags
}

func visibleCommands(commands Commands) Commands {
	visible := make(Commands, 0, len(commands))
	for _, command := range commands {
		if command.Hidden || !command.Runnable() {
			continue
		}
		visible = append(visible, command)
	}
	return visible
}

func printCommands(out io.Writer, title string, commands Commands) {
	commands = visibleCommands(commands)
	if len(commands) == 0 {
		return
	}

	grouped := make([]string, 0)
	byGroup := map[string]Commands{}
	for _, command := range commands {
		group := command.Group
		if group == "" {
			group = "Commands"
		}
		if _, ok := byGroup[group]; !ok {
			grouped = append(grouped, group)
		}
		byGroup[group] = append(byGroup[group], command)
	}
	slices.Sort(grouped)

	fmt.Fprintf(out, "%s:\n", title)
	for _, group := range grouped {
		if len(grouped) > 1 || group != "Commands" {
			fmt.Fprintf(out, "  %s:\n", group)
		}
		maxLen := 0
		for _, command := range byGroup[group] {
			if len(command.Name) > maxLen {
				maxLen = len(command.Name)
			}
		}
		for _, command := range byGroup[group] {
			prefix := "  "
			if len(grouped) > 1 || group != "Commands" {
				prefix = "    "
			}
			fmt.Fprintf(out, "%s%-*s %s\n", prefix, maxLen+2, command.Name, command.Short)
		}
	}
	fmt.Fprintf(out, "\n")
}

func printFlagSections(out io.Writer, title string, flagSet *FlagSet) {
	flags := visibleFlags(flagSet)
	if len(flags) == 0 {
		return
	}

	groupNames := make([]string, 0)
	grouped := map[string][]*Flag{}
	for _, flag := range flags {
		group := flag.Category
		if group == "" {
			group = "Flags"
		}
		if _, ok := grouped[group]; !ok {
			groupNames = append(groupNames, group)
		}
		grouped[group] = append(grouped[group], flag)
	}
	slices.Sort(groupNames)

	fmt.Fprintf(out, "%s:\n", title)
	for _, group := range groupNames {
		if len(groupNames) > 1 || group != "Flags" {
			fmt.Fprintf(out, "  %s:\n", group)
		}

		maxLen := 0
		for _, flag := range grouped[group] {
			if l := len(flagSynopsis(flag)); l > maxLen {
				maxLen = l
			}
		}

		for _, flag := range grouped[group] {
			prefix := "  "
			if len(groupNames) > 1 || group != "Flags" {
				prefix = "    "
			}
			fmt.Fprintf(out, "%s%-*s %s\n", prefix, maxLen+2, flagSynopsis(flag), flagDescription(flag))
		}
	}
	fmt.Fprintf(out, "\n")
}

func flagSynopsis(flag *Flag) string {
	placeholder, _ := UnquoteUsage(flag)
	parts := make([]string, 0, 2)
	if flag.Shorthand != "" {
		parts = append(parts, "-"+flag.Shorthand)
	}
	parts = append(parts, "--"+flag.Name)

	synopsis := strings.Join(parts, ", ")
	if placeholder != "" {
		synopsis += " <" + placeholder + ">"
	}
	return synopsis
}

func flagDescription(flag *Flag) string {
	_, usage := UnquoteUsage(flag)
	annotations := make([]string, 0, 5)
	if len(flag.EnvVars) > 0 {
		annotations = append(annotations, "env: "+strings.Join(flag.EnvVars, ", "))
	}
	if len(flag.ConfigKeys) > 0 {
		annotations = append(annotations, "config: "+strings.Join(flag.ConfigKeys, ", "))
	}
	if len(flag.Enum) > 0 {
		annotations = append(annotations, "choices: "+strings.Join(flag.Enum, ", "))
	}
	if flag.Required {
		annotations = append(annotations, "required")
	}
	if flag.Deprecated != "" {
		annotations = append(annotations, "deprecated: "+flag.Deprecated)
	}
	if flag.Example != "" {
		annotations = append(annotations, "example: "+flag.Example)
	}
	if def := flagDefaultDescription(flag); def != "" {
		annotations = append(annotations, "default: "+def)
	}
	if len(annotations) == 0 {
		return usage
	}
	return usage + " [" + strings.Join(annotations, "] [") + "]"
}

func flagDefaultDescription(flag *Flag) string {
	isZero, err := isZeroValue(flag, flag.DefValue)
	if err != nil || isZero {
		return ""
	}
	if _, ok := flag.Value.(*stringValue); ok {
		return strconv.Quote(flag.DefValue)
	}
	return flag.DefValue
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
