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
	"unsafe"
)

var (
	ErrNotFound = errors.New("not found")

	defaultUsageTemplate = `
{{.Name}} - {{.Short}}

Usage:

  {{if .UsageLine}}{{.UsageLine}}{{else}}{{.Name}} [flags]{{if .Commands}} <command> [subcommand]{{end}} [args]{{end}}

{{if .Long}}{{.Long | trim}}

{{end}}{{if .Commands}}Available Commands:
{{range .Commands}}{{if not .Hidden}}
  {{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}

Use "{{.Name}} help [command]" for more information about a command.
{{end}}`

	DefaultApp = NewApp(filepath.Base(os.Args[0]))
)

// App is the main application container
type App struct {
	Name          string
	Short         string
	Long          string
	Root          *Command
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

	mu          sync.Mutex
	cacheMu     sync.Mutex
	exitStatus  int
	flag        *FlagSet
	configData  map[string]any
	currentRoot *Command

	cachedRootSubCommands    Commands
	cachedRootSubCommandsSig commandsCacheSig
	cachedRootSubCommandsOK  bool
	cachedSyntheticRoot      *Command
	cachedSyntheticRootSig   commandsCacheSig
	cachedSyntheticRootName  string
	cachedSyntheticRootShort string
	cachedSyntheticRootLong  string
	cachedSyntheticRootOK    bool
	cachedRootCommandIndex   map[string]*Command
	cachedRootCommandIndexOK bool
	cachedBuiltinSpecs       []string
	cachedBuiltinSpecsSig    commandsCacheSig
	cachedBuiltinSpecsOK     bool
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

func (a *App) configureFlagSet(flagSet *FlagSet, root *Command, command *Command) {
	if flagSet == nil {
		return
	}
	if a.configEnabled() {
		a.configureConfigFlag(flagSet)
	}
	a.mergeFlags(flagSet, a.SetFlags, true)
	if root != nil {
		a.mergeFlags(flagSet, root.SetFlags, true)
	}
	if command != nil && command != root && command.SetFlags != nil {
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

func (a *App) mergeFlags(dst *FlagSet, register func(f *FlagSet), keepExisting bool) {
	if dst == nil || register == nil {
		return
	}

	tmp := NewFlagSet(dst.Name(), ContinueOnError)
	tmp.SetOutput(io.Discard)
	register(tmp)
	mergeFlagSets(dst, tmp, keepExisting)
}

func mergeFlagSets(dst *FlagSet, src *FlagSet, keepExisting bool) {
	if dst == nil || src == nil {
		return
	}

	src.VisitAll(func(flag *Flag) {
		if _, exists := dst.Lookup(flag.Name); exists {
			if keepExisting {
				return
			}
			panic(dst.sprintf("flag redefined: %s", flag.Name))
		}
		if flag.Shorthand != "" {
			if _, exists := dst.LookupShort(flag.Shorthand); exists {
				if keepExisting {
					return
				}
				panic(dst.sprintf("shorthand redefined: %s", flag.Shorthand))
			}
		}

		cloned := cloneFlag(flag)
		dst.formal = append(dst.formal, cloned)
		dst.sorted = false
	})
}

func cloneFlag(flag *Flag) *Flag {
	if flag == nil {
		return nil
	}
	return &Flag{
		Name:       flag.Name,
		Shorthand:  flag.Shorthand,
		Usage:      flag.Usage,
		Value:      flag.Value,
		DefValue:   flag.DefValue,
		Category:   flag.Category,
		EnvVars:    append([]string(nil), flag.EnvVars...),
		ConfigKeys: append([]string(nil), flag.ConfigKeys...),
		Enum:       append([]string(nil), flag.Enum...),
		Required:   flag.Required,
		Hidden:     flag.Hidden,
		Deprecated: flag.Deprecated,
		Example:    flag.Example,
		Completion: flag.Completion,
		Extensions: cloneExtensions(flag.Extensions),
	}
}

func cloneFlagSetDefinition(src *FlagSet, name string, output io.Writer) *FlagSet {
	flagSet := NewFlagSet(name, ContinueOnError)
	flagSet.SetOutput(output)
	if src == nil || len(src.formal) == 0 {
		return flagSet
	}

	flagSet.formal = src.formal[:len(src.formal):len(src.formal)]
	flagSet.sorted = src.sorted
	return flagSet
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
	var aliasMatch *Command
	for _, cmd := range *c {
		if cmd == nil {
			continue
		}
		if cmd.Name == name {
			return cmd
		}
		if aliasMatch == nil && slices.Contains(cmd.Aliases, name) {
			aliasMatch = cmd
		}
	}

	return aliasMatch
}

// ConfigureFlags sets the app-level global flag registration function.
func (a *App) ConfigureFlags(f func(f *FlagSet)) {
	if a == nil {
		return
	}
	a.SetFlags = f
}

// UseUsageTemplate replaces the app usage template.
func (a *App) UseUsageTemplate(usageTemplate string) {
	if a == nil {
		return
	}
	a.UsageTemplate = usageTemplate
}

// AddCommands appends top-level commands to the app.
func (a *App) AddCommands(cmds ...*Command) {
	if a == nil || len(cmds) == 0 {
		return
	}
	a.Commands = append(a.Commands, cmds...)
}

// SetRootCommand assigns the app root command.
func (a *App) SetRootCommand(root *Command) {
	if a == nil {
		return
	}
	a.Root = root
}

// Execute runs the app with the provided args.
func (a *App) Execute(args []string) error {
	if a == nil {
		return nil
	}
	return a.Run(context.Background(), args)
}

// SetUsageTemplate set value to usageTemplate
func SetUsageTemplate(usageTemplate string) {
	DefaultApp.UseUsageTemplate(usageTemplate)
}

// SetFlags set flags to all commands
func SetFlags(f func(f *FlagSet)) {
	DefaultApp.ConfigureFlags(f)
}

// AddCommands Add Command.
func AddCommands(cmds ...*Command) {
	DefaultApp.AddCommands(cmds...)
}

// Execute func
func Execute() {
	if err := DefaultApp.Execute(os.Args[1:]); err != nil {
		fmt.Fprintf(DefaultApp.Err, "Error: %v\n", err)
		os.Exit(DefaultApp.ExitStatus())
	}
	os.Exit(DefaultApp.ExitStatus())
}

func (a *App) rootCommand() *Command {
	if a.currentRoot != nil {
		return a.currentRoot
	}

	subCommands := a.rootSubCommands()
	if a.Root == nil {
		return a.syntheticRootCommand(subCommands)
	}

	root := &Command{
		Name:        a.Name,
		UsageLine:   a.defaultRootUsageLine(subCommands),
		Short:       a.Short,
		Long:        a.Long,
		SubCommands: subCommands,
		app:         a,
	}
	*root = *a.Root
	root.Name = a.Name
	root.Short = a.Root.Short
	if root.Short == "" {
		root.Short = a.Short
	}
	root.Long = a.Root.Long
	if root.Long == "" {
		root.Long = a.Long
	}
	root.UsageLine = a.Root.UsageLine
	if root.UsageLine == "" {
		root.UsageLine = a.defaultRootUsageLine(subCommands)
	}
	root.SubCommands = subCommands
	root.alias = ""
	root.flag = nil
	root.app = a
	return root
}

func (a *App) syntheticRootCommand(subCommands Commands) *Command {
	sig := makeCommandsCacheSig(nil, nil, a.Commands)

	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()

	if sig == a.cachedSyntheticRootSig &&
		a.cachedSyntheticRootOK &&
		a.cachedSyntheticRootName == a.Name &&
		a.cachedSyntheticRootShort == a.Short &&
		a.cachedSyntheticRootLong == a.Long {
		return a.cachedSyntheticRoot
	}

	root := &Command{
		Name:        a.Name,
		UsageLine:   a.defaultRootUsageLine(subCommands),
		Short:       a.Short,
		Long:        a.Long,
		SubCommands: subCommands,
		app:         a,
	}
	a.cachedSyntheticRoot = root
	a.cachedSyntheticRootSig = sig
	a.cachedSyntheticRootName = a.Name
	a.cachedSyntheticRootShort = a.Short
	a.cachedSyntheticRootLong = a.Long
	a.cachedSyntheticRootOK = true
	return root
}

func (a *App) rootSubCommands() Commands {
	sig := makeCommandsCacheSig(a.Root, a.rootSubCommandsSource(), a.Commands)

	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()

	if sig == a.cachedRootSubCommandsSig && a.cachedRootSubCommandsOK {
		return a.cachedRootSubCommands
	}

	var subCommands Commands
	if a.Root == nil {
		subCommands = a.Commands
	} else {
		subCommands = mergeCommands(a.Root.SubCommands, a.Commands)
	}

	a.cachedRootSubCommands = subCommands
	a.cachedRootSubCommandsSig = sig
	a.cachedRootSubCommandsOK = true
	a.cachedRootCommandIndex = nil
	a.cachedRootCommandIndexOK = false
	a.cachedBuiltinSpecs = nil
	a.cachedBuiltinSpecsOK = false
	a.cachedSyntheticRoot = nil
	a.cachedSyntheticRootOK = false
	return subCommands
}

func (a *App) rootSubCommandsSource() Commands {
	if a.Root == nil {
		return nil
	}
	return a.Root.SubCommands
}

func mergeCommands(primary Commands, secondary Commands) Commands {
	if len(primary) == 0 {
		return secondary
	}
	if len(secondary) == 0 {
		return primary
	}

	merged := make(Commands, 0, len(primary)+len(secondary))
	merged = append(merged, primary...)
	for _, cmd := range secondary {
		if cmd == nil || commandsContainName(merged, cmd.Name) {
			continue
		}
		merged = append(merged, cmd)
	}
	return merged
}

func commandsContainName(commands Commands, name string) bool {
	for _, cmd := range commands {
		if cmd != nil && cmd.Name == name {
			return true
		}
	}
	return false
}

func (a *App) defaultRootUsageLine(subCommands Commands) string {
	if len(subCommands) > 0 {
		return a.Name + " [flags] <command> [subcommand] [args]"
	}
	return a.Name + " [flags] [args]"
}

func (a *App) shouldRunRoot(root *Command, remainingArgs []string) bool {
	if root == nil || !root.Runnable() {
		return false
	}
	if len(remainingArgs) == 0 {
		return true
	}
	if len(root.SubCommands) == 0 {
		return true
	}
	return len(root.Positionals) > 0
}

func (a *App) searchTopLevelCommand(name string) *Command {
	commands := a.rootSubCommands()
	if len(commands) < indexedCommandLookupThreshold {
		return (&commands).Search(name)
	}
	sig := makeCommandsCacheSig(a.Root, a.rootSubCommandsSource(), a.Commands)

	a.cacheMu.Lock()
	if sig != a.cachedRootSubCommandsSig || !a.cachedRootCommandIndexOK {
		a.cachedRootCommandIndex = buildCommandLookup(commands)
		a.cachedRootCommandIndexOK = true
		a.cachedRootSubCommandsSig = sig
	}
	index := a.cachedRootCommandIndex
	a.cacheMu.Unlock()

	return index[name]
}

// Run executes the application
func (a *App) Run(ctx context.Context, args []string) error {
	log.SetFlags(0)
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		a.Usage()
		return nil
	}

	a.flag = nil
	a.configData = nil
	a.exitStatus = 0
	root := a.rootCommand()
	a.currentRoot = root
	defer func() {
		a.currentRoot = nil
	}()
	if a.SetFlags != nil || a.configEnabled() || (root != nil && root.SetFlags != nil) {
		a.flag = a.newRootFlagSetFor(root, a.Err)
		a.flag.Usage = a.Usage
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

	if len(remainingArgs) == 0 {
		if a.shouldRunRoot(root, remainingArgs) {
			return a.runCommand(ctx, root, root, args, nil)
		}
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
	cmd, remainingArgs, err := findCommand(a, nil, root.SubCommands, remainingArgs)

	if err != nil {
		if a.shouldRunRoot(root, remainingArgs) {
			return a.runCommand(ctx, root, root, args, nil)
		}

		suggestions := suggestCommand(name, root.SubCommands)
		if len(suggestions) > 0 {
			return a.fail(nil, ctx, nil, nil, fmt.Errorf("%w, unknown command %q. Did you mean %s?", ErrNotFound, name, strings.Join(suggestions, " or ")), ErrorKindNotFound, 2)
		}
		return a.fail(nil, ctx, nil, nil, fmt.Errorf("%w, unknown command %q", ErrNotFound, name), ErrorKindNotFound, 2)
	}

	return a.runCommand(ctx, root, cmd, remainingArgs, parsedAppFlags)
}

func (a *App) runCommand(ctx context.Context, root *Command, cmd *Command, args []string, inheritedFlags []parsedFlagValue) error {
	cmd.app = a
	startTime := time.Now()

	cmd.flag = a.newCommandFlagSetFor(root, a.flag, cmd, a.Err)
	cmd.flag.Usage = func() {
		if cmd.Name == a.Name {
			a.Usage()
			return
		}
		cmd.Usage()
	}

	if err := cmd.flag.ApplyConfig(a.configData); err != nil {
		return a.fail(cmd, ctx, args, &startTime, err, ErrorKindInvalidArguments, 2)
	}

	if err := cmd.flag.ApplyEnv(); err != nil {
		return a.fail(cmd, ctx, args, &startTime, err, ErrorKindInvalidArguments, 2)
	}

	for _, parsedFlag := range inheritedFlags {
		if err := cmd.flag.Set(parsedFlag.Name, parsedFlag.Value); err != nil {
			return a.fail(cmd, ctx, args, &startTime, err, ErrorKindInvalidArguments, 2)
		}
	}

	if err := cmd.flag.Parse(args); err != nil {
		if errors.Is(err, ErrHelp) {
			return nil
		}
		return a.fail(cmd, ctx, args, &startTime, err, ErrorKindInvalidArguments, 2)
	}

	if err := cmd.flag.Validate(); err != nil {
		return a.fail(cmd, ctx, cmd.flag.Args(), &startTime, err, ErrorKindInvalidArguments, 2)
	}

	if err := cmd.validatePositionals(cmd.flag.Args()); err != nil {
		return a.fail(cmd, ctx, cmd.flag.Args(), &startTime, err, ErrorKindInvalidArguments, 2)
	}

	if !cmd.Runnable() {
		cmd.flag.Usage()
		a.setExitStatus(2)
		return nil
	}

	cmd.flag.WarnDeprecated()
	if cmd.Deprecated != "" {
		fmt.Fprintf(a.Err, "Warning: command %q is deprecated: %s\n", cmd.Name, cmd.Deprecated)
	}

	commandArgs := cmd.flag.Args()
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
	middlewares := joinMiddlewares(a.Middlewares, cmd.Middlewares)

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

func joinMiddlewares(appMiddlewares []Middleware, commandMiddlewares []Middleware) []Middleware {
	if len(appMiddlewares) == 0 {
		return commandMiddlewares
	}
	if len(commandMiddlewares) == 0 {
		return appMiddlewares
	}

	middlewares := make([]Middleware, 0, len(appMiddlewares)+len(commandMiddlewares))
	middlewares = append(middlewares, appMiddlewares...)
	middlewares = append(middlewares, commandMiddlewares...)
	return middlewares
}

type commandsCacheSig struct {
	rootPtr         uintptr
	rootCommandsPtr uintptr
	rootCommandsLen int
	appCommandsPtr  uintptr
	appCommandsLen  int
}

type commandLookupCache struct {
	mu    sync.Mutex
	sig   commandsCacheSig
	index map[string]*Command
	ok    bool
}

var nestedCommandLookupCaches sync.Map

const indexedCommandLookupThreshold = 16

func makeCommandsCacheSig(root *Command, rootCommands Commands, appCommands Commands) commandsCacheSig {
	return commandsCacheSig{
		rootPtr:         commandPointer(root),
		rootCommandsPtr: sliceDataPointer(rootCommands),
		rootCommandsLen: len(rootCommands),
		appCommandsPtr:  sliceDataPointer(appCommands),
		appCommandsLen:  len(appCommands),
	}
}

func commandPointer(cmd *Command) uintptr {
	if cmd == nil {
		return 0
	}
	return uintptr(unsafe.Pointer(cmd))
}

func sliceDataPointer[T any](values []T) uintptr {
	if len(values) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(unsafe.SliceData(values)))
}

func buildCommandLookup(commands Commands) map[string]*Command {
	if len(commands) == 0 {
		return nil
	}

	aliasCount := 0
	for _, cmd := range commands {
		if cmd != nil {
			aliasCount += len(cmd.Aliases)
		}
	}
	index := make(map[string]*Command, len(commands)+aliasCount)
	for _, cmd := range commands {
		if cmd == nil || cmd.Name == "" {
			continue
		}
		if _, exists := index[cmd.Name]; !exists {
			index[cmd.Name] = cmd
		}
	}
	for _, cmd := range commands {
		if cmd == nil {
			continue
		}
		for _, alias := range cmd.Aliases {
			if alias == "" {
				continue
			}
			if _, exists := index[alias]; !exists {
				index[alias] = cmd
			}
		}
	}
	return index
}

func (c *Command) searchSubCommand(name string) *Command {
	if c == nil {
		return nil
	}
	if len(c.SubCommands) == 0 {
		return nil
	}
	if len(c.SubCommands) < indexedCommandLookupThreshold {
		return (&c.SubCommands).Search(name)
	}

	key := commandPointer(c)
	cacheValue, _ := nestedCommandLookupCaches.LoadOrStore(key, &commandLookupCache{})
	cache := cacheValue.(*commandLookupCache)
	sig := makeCommandsCacheSig(c, c.SubCommands, nil)

	cache.mu.Lock()
	if sig != cache.sig || !cache.ok {
		cache.index = buildCommandLookup(c.SubCommands)
		cache.sig = sig
		cache.ok = true
	}
	index := cache.index
	cache.mu.Unlock()

	return index[name]
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
	root := a.rootCommand()
	data := struct {
		Name        string
		Short       string
		Long        string
		UsageLine   string
		Commands    Commands
		Aliases     []string
		Examples    []string
		Positionals []PositionalArg
	}{
		Name:        a.Name,
		Short:       root.Short,
		Long:        root.Long,
		UsageLine:   root.UsageLine,
		Commands:    root.SubCommands,
		Aliases:     root.Aliases,
		Examples:    root.Examples,
		Positionals: root.Positionals,
	}
	bw := bufio.NewWriter(a.Err)
	runTemplate(bw, a.UsageTemplate, data)
	if a.flag != nil {
		printFlagSections(bw, "Global Flags", a.flag)
	}
	printPositionals(bw, root.Positionals)
	if len(root.Examples) > 0 {
		fmt.Fprintf(bw, "Examples:\n")
		for _, example := range root.Examples {
			fmt.Fprintf(bw, "  %s\n", example)
		}
		fmt.Fprintf(bw, "\n")
	}
	bw.Flush()
}

func (a *App) help(args []string) error {
	if len(args) == 0 {
		a.Usage()
		return nil
	}

	root := a.rootCommand()
	cmd, _, err := findCommand(a, nil, root.SubCommands, args)
	if err != nil {
		return err
	}
	cmd.app = a
	rootFlags := a.newRootFlagSetFor(root, a.Err)
	cmd.flag = a.newCommandFlagSetFor(root, rootFlags, cmd, a.Err)
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
func findCommand(app *App, parent *Command, cmds Commands, args []string) (*Command, []string, error) {

	if len(args) == 0 {
		return nil, nil, fmt.Errorf("%w, no command provided", ErrNotFound)
	}

	name := args[0]

	var cmd *Command
	switch {
	case parent != nil:
		cmd = parent.searchSubCommand(name)
	case app != nil:
		if len(cmds) < indexedCommandLookupThreshold {
			cmd = cmds.Search(name)
		} else {
			cmd = app.searchTopLevelCommand(name)
		}
	default:
		cmd = cmds.Search(name)
	}

	if cmd == nil {
		return nil, nil, fmt.Errorf("%w, unknown command %q", ErrNotFound, name)
	}

	cmd.alias = name

	if len(args) > 1 && len(cmd.SubCommands) > 0 {

		subCmd, remainingArgs, err := findCommand(nil, cmd, cmd.SubCommands, args[1:])

		if err == nil {
			return subCmd, remainingArgs, nil
		}
		if !cmd.Runnable() {
			return nil, nil, err
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
		if command.Hidden {
			continue
		}
		if !command.Runnable() && len(command.SubCommands) == 0 {
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
	t := parseTemplate("top", text)
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

func parseTemplate(name string, text string) *template.Template {
	t := template.New(name)
	t.Funcs(template.FuncMap{
		"trim":       strings.TrimSpace,
		"capitalize": capitalize,
	})
	return template.Must(t.Parse(text))
}
