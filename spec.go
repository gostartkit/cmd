package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

var (
	sourceOrderCLIDefault          = []string{"cli", "default"}
	sourceOrderCLIEnvDefault       = []string{"cli", "env", "default"}
	sourceOrderCLIConfigDefault    = []string{"cli", "config", "default"}
	sourceOrderCLIEnvConfigDefault = []string{"cli", "env", "config", "default"}
)

type AppSpec struct {
	SchemaVersion     string            `json:"schema_version"`
	Name              string            `json:"name"`
	Short             string            `json:"short"`
	Surface           string            `json:"surface,omitempty"`
	AvailableSurfaces []string          `json:"available_surfaces,omitempty"`
	Builtins          []string          `json:"builtins,omitempty"`
	Capabilities      CapabilitySpec    `json:"capabilities"`
	Config            ConfigRuntimeSpec `json:"config"`
	Hooks             HookSpec          `json:"hooks,omitempty"`
	HasMiddleware     bool              `json:"has_middleware,omitempty"`
	HasObservers      bool              `json:"has_observers,omitempty"`
	Extensions        map[string]any    `json:"extensions,omitempty"`
	Root              *CommandSpec      `json:"root,omitempty"`
	GlobalFlags       []FlagSpec        `json:"global_flags,omitempty"`
	Commands          []CommandSpec     `json:"commands"`
}

type CommandSpec struct {
	ID            string           `json:"id,omitempty"`
	HandlerID     string           `json:"handler_id,omitempty"`
	Path          []string         `json:"path,omitempty"`
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
	ID                 string         `json:"id,omitempty"`
	Name               string         `json:"name"`
	Usage              string         `json:"usage,omitempty"`
	Required           bool           `json:"required,omitempty"`
	Variadic           bool           `json:"variadic,omitempty"`
	Kind               string         `json:"kind,omitempty"`
	Enum               []string       `json:"enum,omitempty"`
	Example            string         `json:"example,omitempty"`
	CompletionKey      string         `json:"completion_key,omitempty"`
	SupportsCompletion bool           `json:"supports_completion,omitempty"`
	Extensions         map[string]any `json:"extensions,omitempty"`
}

type FlagSpec struct {
	ID                 string         `json:"id,omitempty"`
	Name               string         `json:"name"`
	Shorthand          string         `json:"shorthand,omitempty"`
	Type               string         `json:"type,omitempty"`
	Kind               string         `json:"kind,omitempty"`
	Usage              string         `json:"usage,omitempty"`
	Default            string         `json:"default,omitempty"`
	Category           string         `json:"category,omitempty"`
	EnvVars            []string       `json:"env_vars,omitempty"`
	ConfigKeys         []string       `json:"config_keys,omitempty"`
	SourceOrder        []string       `json:"source_order,omitempty"`
	Enum               []string       `json:"enum,omitempty"`
	Required           bool           `json:"required,omitempty"`
	Repeatable         bool           `json:"repeatable,omitempty"`
	Hidden             bool           `json:"hidden,omitempty"`
	Deprecated         string         `json:"deprecated,omitempty"`
	Example            string         `json:"example,omitempty"`
	CompletionKey      string         `json:"completion_key,omitempty"`
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
	SurfaceOverrides   bool `json:"surface_overrides"`
	StableIDs          bool `json:"stable_ids"`
	SemanticKinds      bool `json:"semantic_kinds"`
	RepeatableFlags    bool `json:"repeatable_flags"`
	CompletionKeys     bool `json:"completion_keys"`
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
	return a.SpecFor("")
}

func (a *App) AvailableSurfaces() []Surface {
	names := a.availableSurfaces(a.rootCommand())
	if len(names) == 0 {
		return nil
	}
	surfaces := make([]Surface, 0, len(names))
	for _, name := range names {
		surfaces = append(surfaces, Surface(name))
	}
	return surfaces
}

func (a *App) SpecFor(surface Surface) AppSpec {
	root := a.rootCommand()
	var globalFlagSpecs []FlagSpec
	surfaceName := strings.TrimSpace(string(surface))
	spec := AppSpec{
		SchemaVersion:     "v2",
		Name:              a.Name,
		Short:             root.Short,
		Surface:           surfaceName,
		AvailableSurfaces: a.availableSurfaces(root),
		Builtins:          a.builtinSpecsForCommands(root.SubCommands),
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
			SurfaceOverrides:   true,
			StableIDs:          true,
			SemanticKinds:      true,
			RepeatableFlags:    true,
			CompletionKeys:     true,
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
	if rootDef, ok := a.rootFlagDefinition(root); ok && rootDef != nil {
		globalFlagSpecs = flagSetDefSpec(nil, rootDef, surfaceName)
		spec.GlobalFlags = globalFlagSpecs
	} else {
		rootFlags := a.newRootFlagSetFor(root, io.Discard)
		if rootFlags != nil {
			globalFlagSpecs = flagSetSpec(nil, rootFlags, surfaceName)
			spec.GlobalFlags = globalFlagSpecs
		}
	}
	rootSpec := a.commandSpec(root, root, nil, globalFlagSpecs, surfaceName)
	rootSpec.Name = a.Name
	if rootSpec.ID == "" {
		rootSpec.ID = a.Name
	}
	if root.Runnable() && rootSpec.HandlerID == "" {
		rootSpec.HandlerID = a.Name
	}
	spec.Root = &rootSpec
	for _, command := range root.SubCommands {
		spec.Commands = append(spec.Commands, a.commandSpec(root, command, nil, globalFlagSpecs, surfaceName))
	}
	return spec
}

func (a *App) commandSpec(root *Command, cmd *Command, prefix []string, globalFlagSpecs []FlagSpec, surface string) CommandSpec {
	path := prefix
	if cmd != nil && cmd != root {
		path = append(append([]string(nil), prefix...), cmd.Name)
	}
	spec := CommandSpec{
		ID:            commandID(cmd, path),
		HandlerID:     commandHandlerID(cmd, path),
		Path:          append([]string(nil), path...),
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
	}
	positionals := surfacePositionals(cmd.Positionals, surface, cmd.surfaceOverride(surface))
	spec.Positionals = positionalSpecs(path, positionals, surface)
	if cmd == root {
		if len(globalFlagSpecs) > 0 {
			spec.Flags = append([]FlagSpec(nil), globalFlagSpecs...)
		}
	} else {
		if def, ok := a.commandFlagDefinition(root, cmd); ok && def != nil {
			spec.Flags = flagSetDefSpec(path, def, surface)
		} else {
			spec.Flags = mergeCommandFlagSpecs(path, surface, globalFlagSpecs, a.localCommandFlagSpecs(path, cmd, surface))
		}
	}
	spec = applyCommandSurface(spec, cmd.surfaceOverride(surface))
	if len(cmd.SubCommands) > 0 {
		spec.SubCommands = make([]CommandSpec, 0, len(cmd.SubCommands))
		for _, subCommand := range cmd.SubCommands {
			spec.SubCommands = append(spec.SubCommands, a.commandSpec(root, subCommand, path, globalFlagSpecs, surface))
		}
	}
	return spec
}

func (a *App) localCommandFlagSpecs(path []string, cmd *Command, surface string) []FlagSpec {
	if cmd == nil || cmd.SetFlags == nil {
		return nil
	}
	flagSet := NewFlagSet(cmd.Name, ContinueOnError)
	flagSet.SetOutput(io.Discard)
	cmd.SetFlags(flagSet)
	return flagSetSpec(path, flagSet, surface)
}

func mergeCommandFlagSpecs(path []string, surface string, global []FlagSpec, local []FlagSpec) []FlagSpec {
	switch {
	case len(global) == 0:
		return append([]FlagSpec(nil), local...)
	case len(local) == 0:
		return append([]FlagSpec(nil), global...)
	}

	merged := make([]FlagSpec, 0, len(global)+len(local))
	i, j := 0, 0
	for i < len(global) && j < len(local) {
		if strings.Compare(global[i].Name, local[j].Name) <= 0 {
			merged = append(merged, global[i])
			i++
			continue
		}
		merged = append(merged, local[j])
		j++
	}
	merged = append(merged, global[i:]...)
	merged = append(merged, local[j:]...)
	return merged
}

func positionalSpecs(path []string, positionals []PositionalArg, surface string) []PositionalSpec {
	if len(positionals) == 0 {
		return nil
	}
	specs := make([]PositionalSpec, 0, len(positionals))
	for index, positional := range positionals {
		specs = append(specs, positionalSpec(path, index, positional, surface))
	}
	return specs
}

func flagSetSpec(path []string, flagSet *FlagSet, surface string) []FlagSpec {
	if flagSet == nil {
		return nil
	}
	if flagSet.def != nil {
		flags := make([]FlagSpec, 0, len(flagSet.def.Flags))
		for _, defined := range flagSet.def.Flags {
			flags = append(flags, flagSpecFromDef(path, defined, surface))
		}
		return flags
	}
	flags := make([]FlagSpec, 0, len(flagSet.formal))
	flagSet.VisitAll(func(flag *Flag) {
		flags = append(flags, flagSpec(path, flag, surface))
	})
	return flags
}

func flagSetDefSpec(path []string, def *flagSetDef, surface string) []FlagSpec {
	if def == nil || len(def.Flags) == 0 {
		return nil
	}
	flags := make([]FlagSpec, 0, len(def.Flags))
	for _, defined := range def.Flags {
		flags = append(flags, flagSpecFromDef(path, defined, surface))
	}
	return flags
}

func flagSpec(path []string, flag *Flag, surface string) FlagSpec {
	flagType, usage := UnquoteUsage(flag)
	spec := FlagSpec{
		ID:            flagID(path, flag),
		Name:          flag.Name,
		Shorthand:     flag.Shorthand,
		Type:          flagType,
		Kind:          defaultFlagKind(flag.Kind, flagType),
		Usage:         usage,
		Default:       flag.DefValue,
		Category:      flag.Category,
		EnvVars:       append([]string(nil), flag.EnvVars...),
		ConfigKeys:    append([]string(nil), flag.ConfigKeys...),
		SourceOrder:   sourceOrder(flag),
		Enum:          append([]string(nil), flag.Enum...),
		Required:      flag.Required,
		Repeatable:    flag.Repeatable,
		Hidden:        flag.Hidden,
		Deprecated:    flag.Deprecated,
		Example:       flag.Example,
		CompletionKey: strings.TrimSpace(flag.CompletionKey),
		Extensions:    cloneExtensions(flag.Extensions),
	}
	spec = applyFlagSurface(spec, flag.surfaceOverride(surface))
	spec.SupportsCompletion = flag.Completion != nil || len(spec.Enum) > 0 || spec.CompletionKey != ""
	return spec
}

func flagSpecFromDef(path []string, def *flagDef, surface string) FlagSpec {
	if def == nil {
		return FlagSpec{}
	}
	flagType, usage := unquoteUsageForKind(def.Usage, def.ValueKind, nil)
	spec := FlagSpec{
		ID:            flagDefID(path, def),
		Name:          def.Name,
		Shorthand:     def.Shorthand,
		Type:          flagType,
		Kind:          defaultFlagKind(def.Kind, flagType),
		Usage:         usage,
		Default:       def.DefValue,
		Category:      def.Category,
		EnvVars:       append([]string(nil), def.EnvVars...),
		ConfigKeys:    append([]string(nil), def.ConfigKeys...),
		SourceOrder:   sourceOrderForDef(def),
		Enum:          append([]string(nil), def.Enum...),
		Required:      def.Required,
		Repeatable:    def.Repeatable,
		Hidden:        def.Hidden,
		Deprecated:    def.Deprecated,
		Example:       def.Example,
		CompletionKey: strings.TrimSpace(def.CompletionKey),
		Extensions:    cloneExtensions(def.Extensions),
	}
	spec = applyFlagSurface(spec, def.surfaceOverride(surface))
	spec.SupportsCompletion = def.Completion != nil || len(spec.Enum) > 0 || spec.CompletionKey != ""
	return spec
}

func positionalSpec(path []string, index int, positional PositionalArg, surface string) PositionalSpec {
	spec := PositionalSpec{
		ID:            positionalID(path, index, positional),
		Name:          positional.Name,
		Usage:         positional.Usage,
		Required:      positional.Required,
		Variadic:      positional.Variadic,
		Kind:          defaultPositionalKind(positional.Kind, positional.Enum),
		Enum:          append([]string(nil), positional.Enum...),
		Example:       positional.Example,
		CompletionKey: strings.TrimSpace(positional.CompletionKey),
		Extensions:    cloneExtensions(positional.Extensions),
	}
	spec = applyPositionalSurface(spec, positional.surfaceOverride(surface))
	spec.SupportsCompletion = positional.Completion != nil || len(spec.Enum) > 0 || spec.CompletionKey != ""
	return spec
}

func (a *App) availableSurfaces(root *Command) []string {
	if root == nil {
		return nil
	}
	seen := make(map[string]struct{})
	if def, ok := a.rootFlagDefinition(root); ok && def != nil {
		collectFlagDefSurfaces(seen, def)
	}
	collectCommandSurfaces(seen, root)
	if len(seen) == 0 {
		return nil
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func collectCommandSurfaces(dst map[string]struct{}, cmd *Command) {
	if cmd == nil {
		return
	}
	for surface := range cmd.Surfaces {
		if name := strings.TrimSpace(string(surface)); name != "" {
			dst[name] = struct{}{}
		}
	}
	for _, positional := range cmd.Positionals {
		for surface := range positional.Surfaces {
			if name := strings.TrimSpace(string(surface)); name != "" {
				dst[name] = struct{}{}
			}
		}
	}
	if def, ok := cmd.localFlagDefinition(); ok && def != nil {
		collectFlagDefSurfaces(dst, def)
	}
	for _, sub := range cmd.SubCommands {
		collectCommandSurfaces(dst, sub)
	}
}

func collectFlagDefSurfaces(dst map[string]struct{}, def *flagSetDef) {
	if def == nil {
		return
	}
	for _, flag := range def.Flags {
		for surface := range flag.Surfaces {
			if name := strings.TrimSpace(string(surface)); name != "" {
				dst[name] = struct{}{}
			}
		}
	}
}

func surfacePositionals(base []PositionalArg, surface string, override *CommandSurface) []PositionalArg {
	if override != nil && override.ReplacePositionals {
		return clonePositionals(override.Positionals)
	}
	return clonePositionals(base)
}

func applyCommandSurface(spec CommandSpec, override *CommandSurface) CommandSpec {
	if override == nil {
		return spec
	}
	if override.UsageLine != "" {
		spec.UsageLine = override.UsageLine
	}
	if override.Short != "" {
		spec.Short = override.Short
	}
	if override.Long != "" {
		spec.Long = override.Long
	}
	if override.Group != "" {
		spec.Group = override.Group
	}
	if override.Deprecated != "" {
		spec.Deprecated = override.Deprecated
	}
	if override.Hidden != nil {
		spec.Hidden = *override.Hidden
	}
	if len(override.Aliases) > 0 {
		spec.Aliases = append([]string(nil), override.Aliases...)
	}
	if len(override.Examples) > 0 {
		spec.Examples = append([]string(nil), override.Examples...)
	}
	if len(override.Extensions) > 0 {
		spec.Extensions = cloneExtensions(override.Extensions)
	}
	return spec
}

func applyPositionalSurface(spec PositionalSpec, override *PositionalSurface) PositionalSpec {
	if override == nil {
		return spec
	}
	if override.Name != "" {
		spec.Name = override.Name
	}
	if override.Usage != "" {
		spec.Usage = override.Usage
	}
	if override.Required != nil {
		spec.Required = *override.Required
	}
	if override.Variadic != nil {
		spec.Variadic = *override.Variadic
	}
	if len(override.Enum) > 0 {
		spec.Enum = append([]string(nil), override.Enum...)
	}
	if override.Example != "" {
		spec.Example = override.Example
	}
	if override.Kind != "" {
		spec.Kind = strings.TrimSpace(override.Kind)
	}
	if override.CompletionKey != "" {
		spec.CompletionKey = strings.TrimSpace(override.CompletionKey)
	}
	if len(override.Extensions) > 0 {
		spec.Extensions = cloneExtensions(override.Extensions)
	}
	return spec
}

func applyFlagSurface(spec FlagSpec, override *FlagSurface) FlagSpec {
	if override == nil {
		return spec
	}
	if override.Usage != "" {
		spec.Usage = override.Usage
	}
	if override.Default != "" {
		spec.Default = override.Default
	}
	if override.Category != "" {
		spec.Category = override.Category
	}
	if len(override.Enum) > 0 {
		spec.Enum = append([]string(nil), override.Enum...)
	}
	if override.Required != nil {
		spec.Required = *override.Required
	}
	if override.Repeatable != nil {
		spec.Repeatable = *override.Repeatable
	}
	if override.Hidden != nil {
		spec.Hidden = *override.Hidden
	}
	if override.Deprecated != "" {
		spec.Deprecated = override.Deprecated
	}
	if override.Example != "" {
		spec.Example = override.Example
	}
	if override.Kind != "" {
		spec.Kind = strings.TrimSpace(override.Kind)
	}
	if override.CompletionKey != "" {
		spec.CompletionKey = strings.TrimSpace(override.CompletionKey)
	}
	if len(override.Extensions) > 0 {
		spec.Extensions = cloneExtensions(override.Extensions)
	}
	return spec
}

func commandID(cmd *Command, path []string) string {
	if cmd != nil && strings.TrimSpace(cmd.ID) != "" {
		return strings.TrimSpace(cmd.ID)
	}
	return strings.TrimSpace(strings.Join(path, " "))
}

func commandHandlerID(cmd *Command, path []string) string {
	if cmd != nil && strings.TrimSpace(cmd.HandlerID) != "" {
		return strings.TrimSpace(cmd.HandlerID)
	}
	return strings.TrimSpace(strings.Join(path, "."))
}

func positionalID(path []string, index int, positional PositionalArg) string {
	if strings.TrimSpace(positional.ID) != "" {
		return strings.TrimSpace(positional.ID)
	}
	key := strings.TrimSpace(strings.Join(path, " "))
	if key == "" {
		return positional.Name
	}
	return fmt.Sprintf("%s#%d", key, index)
}

func flagID(path []string, flag *Flag) string {
	if flag != nil && strings.TrimSpace(flag.ID) != "" {
		return strings.TrimSpace(flag.ID)
	}
	key := strings.TrimSpace(strings.Join(path, " "))
	if key == "" || flag == nil {
		if flag == nil {
			return ""
		}
		return flag.Name
	}
	return key + "#" + flag.Name
}

func flagDefID(path []string, def *flagDef) string {
	if def != nil && strings.TrimSpace(def.ID) != "" {
		return strings.TrimSpace(def.ID)
	}
	key := strings.TrimSpace(strings.Join(path, " "))
	if key == "" || def == nil {
		if def == nil {
			return ""
		}
		return def.Name
	}
	return key + "#" + def.Name
}

func defaultPositionalKind(kind string, enum []string) string {
	if trimmed := strings.TrimSpace(kind); trimmed != "" {
		return trimmed
	}
	if len(enum) > 0 {
		return "enum"
	}
	return "string"
}

func defaultFlagKind(kind string, flagType string) string {
	if trimmed := strings.TrimSpace(kind); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(flagType); trimmed != "" {
		return trimmed
	}
	return "string"
}

func (c *Command) surfaceOverride(surface string) *CommandSurface {
	if c == nil || len(c.Surfaces) == 0 || surface == "" {
		return nil
	}
	override, ok := c.Surfaces[Surface(surface)]
	if !ok {
		return nil
	}
	cloned := cloneCommandSurface(override)
	return &cloned
}

func (p PositionalArg) surfaceOverride(surface string) *PositionalSurface {
	if len(p.Surfaces) == 0 || surface == "" {
		return nil
	}
	override, ok := p.Surfaces[Surface(surface)]
	if !ok {
		return nil
	}
	cloned := clonePositionalSurface(override)
	return &cloned
}

func (f *Flag) surfaceOverride(surface string) *FlagSurface {
	if f == nil || len(f.Surfaces) == 0 || surface == "" {
		return nil
	}
	override, ok := f.Surfaces[Surface(surface)]
	if !ok {
		return nil
	}
	cloned := cloneFlagSurface(override)
	return &cloned
}

func (d *flagDef) surfaceOverride(surface string) *FlagSurface {
	if d == nil || len(d.Surfaces) == 0 || surface == "" {
		return nil
	}
	override, ok := d.Surfaces[Surface(surface)]
	if !ok {
		return nil
	}
	cloned := cloneFlagSurface(override)
	return &cloned
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

func sourceOrderForDef(def *flagDef) []string {
	hasEnv := len(def.EnvVars) > 0
	hasConfig := len(def.ConfigKeys) > 0
	switch {
	case hasEnv && hasConfig:
		return sourceOrderCLIEnvConfigDefault
	case hasEnv:
		return sourceOrderCLIEnvDefault
	case hasConfig:
		return sourceOrderCLIConfigDefault
	default:
		return sourceOrderCLIDefault
	}
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
