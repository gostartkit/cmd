package cmd

import (
	"encoding/json"
	"fmt"
	"io"
)

type AppSpec struct {
	SchemaVersion string            `json:"schema_version"`
	Name          string            `json:"name"`
	Short         string            `json:"short"`
	Builtins      []string          `json:"builtins,omitempty"`
	Capabilities  CapabilitySpec    `json:"capabilities"`
	Config        ConfigRuntimeSpec `json:"config"`
	Hooks         HookSpec          `json:"hooks,omitempty"`
	HasMiddleware bool              `json:"has_middleware,omitempty"`
	HasObservers  bool              `json:"has_observers,omitempty"`
	Extensions    map[string]any    `json:"extensions,omitempty"`
	Root          *CommandSpec      `json:"root,omitempty"`
	GlobalFlags   []FlagSpec        `json:"global_flags,omitempty"`
	Commands      []CommandSpec     `json:"commands"`
}

type CommandSpec struct {
	Name          string           `json:"name"`
	Aliases       []string         `json:"aliases,omitempty"`
	UsageLine     string           `json:"usage_line,omitempty"`
	Short         string           `json:"short,omitempty"`
	Long          string           `json:"long,omitempty"`
	Group         string           `json:"group,omitempty"`
	Deprecated    string           `json:"deprecated,omitempty"`
	Hidden        bool             `json:"hidden,omitempty"`
	Runnable      bool             `json:"runnable,omitempty"`
	Examples      []string         `json:"examples,omitempty"`
	Hooks         HookSpec         `json:"hooks,omitempty"`
	HasMiddleware bool             `json:"has_middleware,omitempty"`
	HasObservers  bool             `json:"has_observers,omitempty"`
	Extensions    map[string]any   `json:"extensions,omitempty"`
	Positionals   []PositionalSpec `json:"positionals,omitempty"`
	Flags         []FlagSpec       `json:"flags,omitempty"`
	SubCommands   []CommandSpec    `json:"subcommands,omitempty"`
}

type PositionalSpec struct {
	Name               string         `json:"name"`
	Usage              string         `json:"usage,omitempty"`
	Required           bool           `json:"required,omitempty"`
	Variadic           bool           `json:"variadic,omitempty"`
	Enum               []string       `json:"enum,omitempty"`
	Example            string         `json:"example,omitempty"`
	SupportsCompletion bool           `json:"supports_completion,omitempty"`
	Extensions         map[string]any `json:"extensions,omitempty"`
}

type FlagSpec struct {
	Name               string         `json:"name"`
	Shorthand          string         `json:"shorthand,omitempty"`
	Type               string         `json:"type,omitempty"`
	Usage              string         `json:"usage,omitempty"`
	Default            string         `json:"default,omitempty"`
	Category           string         `json:"category,omitempty"`
	EnvVars            []string       `json:"env_vars,omitempty"`
	ConfigKeys         []string       `json:"config_keys,omitempty"`
	SourceOrder        []string       `json:"source_order,omitempty"`
	Enum               []string       `json:"enum,omitempty"`
	Required           bool           `json:"required,omitempty"`
	Hidden             bool           `json:"hidden,omitempty"`
	Deprecated         string         `json:"deprecated,omitempty"`
	Example            string         `json:"example,omitempty"`
	SupportsCompletion bool           `json:"supports_completion,omitempty"`
	Extensions         map[string]any `json:"extensions,omitempty"`
}

type HookSpec struct {
	BeforeRun bool `json:"before_run,omitempty"`
	AfterRun  bool `json:"after_run,omitempty"`
	OnError   bool `json:"on_error,omitempty"`
}

type CapabilitySpec struct {
	GlobalFlags        bool `json:"global_flags"`
	InterspersedFlags  bool `json:"interspersed_flags"`
	EnvBinding         bool `json:"env_binding"`
	ConfigBinding      bool `json:"config_binding"`
	ShellCompletion    bool `json:"shell_completion"`
	SpecExport         bool `json:"spec_export"`
	DocsExport         bool `json:"docs_export"`
	ValueCompletion    bool `json:"value_completion"`
	Positionals        bool `json:"positionals"`
	LifecycleHooks     bool `json:"lifecycle_hooks"`
	Middleware         bool `json:"middleware"`
	Observers          bool `json:"observers"`
	ErrorNormalization bool `json:"error_normalization"`
}

type ConfigRuntimeSpec struct {
	Enabled   bool     `json:"enabled"`
	FlagName  string   `json:"flag_name,omitempty"`
	Shorthand string   `json:"shorthand,omitempty"`
	EnvVars   []string `json:"env_vars,omitempty"`
}

func (a *App) runSpec(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: %s spec [json]", a.Name)
	}
	if len(args) == 1 && args[0] != "json" {
		return fmt.Errorf("usage: %s spec [json]", a.Name)
	}

	out := a.Out
	if out == nil {
		out = io.Discard
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(a.Spec())
}

func (a *App) Spec() AppSpec {
	root := a.rootCommand()
	rootFlags := a.newRootFlagSetFor(root, io.Discard)
	spec := AppSpec{
		SchemaVersion: "v2",
		Name:          a.Name,
		Short:         root.Short,
		Builtins:      a.builtinSpecsForCommands(root.SubCommands),
		Capabilities: CapabilitySpec{
			GlobalFlags:        true,
			InterspersedFlags:  true,
			EnvBinding:         true,
			ConfigBinding:      true,
			ShellCompletion:    true,
			SpecExport:         true,
			DocsExport:         true,
			ValueCompletion:    true,
			Positionals:        true,
			LifecycleHooks:     true,
			Middleware:         true,
			Observers:          true,
			ErrorNormalization: true,
		},
		Config: ConfigRuntimeSpec{
			Enabled: a.configEnabled(),
		},
		Hooks: HookSpec{
			BeforeRun: a.BeforeRun != nil,
			AfterRun:  a.AfterRun != nil,
			OnError:   a.OnError != nil,
		},
		HasMiddleware: len(a.Middlewares) > 0,
		HasObservers:  len(a.Observers) > 0,
		Extensions:    cloneExtensions(a.Extensions),
		Commands:      make([]CommandSpec, 0, len(root.SubCommands)),
	}
	if a.configEnabled() {
		spec.Config.FlagName = a.ConfigFlag.Name
		spec.Config.Shorthand = a.ConfigFlag.Shorthand
		spec.Config.EnvVars = append([]string(nil), a.ConfigFlag.EnvVars...)
	}
	if rootFlags != nil {
		spec.GlobalFlags = flagSetSpec(rootFlags)
	}
	rootSpec := a.commandSpec(root, root, rootFlags)
	rootSpec.Name = a.Name
	rootSpec.Flags = append([]FlagSpec(nil), spec.GlobalFlags...)
	spec.Root = &rootSpec
	for _, command := range root.SubCommands {
		spec.Commands = append(spec.Commands, a.commandSpec(root, command, rootFlags))
	}
	return spec
}

func (a *App) commandSpec(root *Command, cmd *Command, rootFlags *FlagSet) CommandSpec {
	spec := CommandSpec{
		Name:          cmd.Name,
		Aliases:       cmd.Aliases,
		UsageLine:     cmd.UsageLine,
		Short:         cmd.Short,
		Long:          cmd.Long,
		Group:         cmd.Group,
		Deprecated:    cmd.Deprecated,
		Hidden:        cmd.Hidden,
		Runnable:      cmd.Runnable(),
		Examples:      append([]string(nil), cmd.Examples...),
		Hooks:         hookSpec(cmd.BeforeRun, cmd.AfterRun, cmd.OnError),
		HasMiddleware: len(cmd.Middlewares) > 0,
		HasObservers:  len(cmd.Observers) > 0,
		Extensions:    cloneExtensions(cmd.Extensions),
		Positionals:   positionalSpecs(cmd.Positionals),
	}
	if localFlags := a.newCommandFlagSetFor(root, rootFlags, cmd, io.Discard); localFlags != nil {
		spec.Flags = flagSetSpec(localFlags)
	}
	if len(cmd.SubCommands) > 0 {
		spec.SubCommands = make([]CommandSpec, 0, len(cmd.SubCommands))
		for _, subCommand := range cmd.SubCommands {
			spec.SubCommands = append(spec.SubCommands, a.commandSpec(root, subCommand, rootFlags))
		}
	}
	return spec
}

func positionalSpecs(positionals []PositionalArg) []PositionalSpec {
	if len(positionals) == 0 {
		return nil
	}
	specs := make([]PositionalSpec, 0, len(positionals))
	for _, positional := range positionals {
		specs = append(specs, PositionalSpec{
			Name:               positional.Name,
			Usage:              positional.Usage,
			Required:           positional.Required,
			Variadic:           positional.Variadic,
			Enum:               append([]string(nil), positional.Enum...),
			Example:            positional.Example,
			SupportsCompletion: positional.Completion != nil || len(positional.Enum) > 0,
			Extensions:         cloneExtensions(positional.Extensions),
		})
	}
	return specs
}

func flagSetSpec(flagSet *FlagSet) []FlagSpec {
	flags := make([]FlagSpec, 0)
	flagSet.VisitAll(func(flag *Flag) {
		flags = append(flags, flagSpec(flag))
	})
	return flags
}

func flagSpec(flag *Flag) FlagSpec {
	flagType, usage := UnquoteUsage(flag)
	return FlagSpec{
		Name:               flag.Name,
		Shorthand:          flag.Shorthand,
		Type:               flagType,
		Usage:              usage,
		Default:            flag.DefValue,
		Category:           flag.Category,
		EnvVars:            append([]string(nil), flag.EnvVars...),
		ConfigKeys:         append([]string(nil), flag.ConfigKeys...),
		SourceOrder:        sourceOrder(flag),
		Enum:               append([]string(nil), flag.Enum...),
		Required:           flag.Required,
		Hidden:             flag.Hidden,
		Deprecated:         flag.Deprecated,
		Example:            flag.Example,
		SupportsCompletion: flag.Completion != nil || len(flag.Enum) > 0,
		Extensions:         cloneExtensions(flag.Extensions),
	}
}

func sourceOrder(flag *Flag) []string {
	order := []string{"cli"}
	if len(flag.EnvVars) > 0 {
		order = append(order, "env")
	}
	if len(flag.ConfigKeys) > 0 {
		order = append(order, "config")
	}
	order = append(order, "default")
	return order
}

func hookSpec(before BeforeHook, after AfterHook, onError ErrorHook) HookSpec {
	return HookSpec{
		BeforeRun: before != nil,
		AfterRun:  after != nil,
		OnError:   onError != nil,
	}
}

func (a *App) builtinSpecs() []string {
	return a.builtinSpecsForCommands(a.rootSubCommands())
}

func (a *App) builtinSpecsForCommands(commands Commands) []string {
	sig := makeCommandsCacheSig(a.Root, a.rootSubCommandsSource(), a.Commands)

	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()

	if sig == a.cachedBuiltinSpecsSig && a.cachedBuiltinSpecsOK {
		return a.cachedBuiltinSpecs
	}

	builtins := builtinSpecsFor(commands)
	a.cachedBuiltinSpecs = builtins
	a.cachedBuiltinSpecsSig = sig
	a.cachedBuiltinSpecsOK = true
	return builtins
}

func builtinSpecsFor(commands Commands) []string {
	return appendBuiltinSpecsFor(nil, commands)
}

func appendBuiltinSpecsFor(dst []string, commands Commands) []string {
	dst = append(dst, "help")
	if commands.Search("completion") == nil {
		dst = append(dst, "completion")
	}
	if commands.Search("spec") == nil {
		dst = append(dst, "spec")
	}
	if commands.Search("docs") == nil {
		dst = append(dst, "docs")
	}
	return dst
}
