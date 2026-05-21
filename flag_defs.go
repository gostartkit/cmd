package cmd

import (
	"encoding"
	"io"
	"reflect"
	"strconv"
	"time"
)

type flagValueFactory func() Value

type flagValueKind uint8

const (
	flagValueKindUnknown flagValueKind = iota
	flagValueKindBool
	flagValueKindInt
	flagValueKindInt64
	flagValueKindUint
	flagValueKindUint64
	flagValueKindString
	flagValueKindFloat64
	flagValueKindDuration
	flagValueKindText
	flagValueKindFunc
	flagValueKindBoolFunc
)

type flagDef struct {
	Name          string
	Shorthand     string
	Usage         string
	DefValue      string
	ID            string
	Kind          string
	Category      string
	EnvVars       []string
	ConfigKeys    []string
	Enum          []string
	Required      bool
	Repeatable    bool
	Hidden        bool
	Deprecated    string
	Example       string
	CompletionKey string
	Completion    CompletionFunc
	Extensions    map[string]any
	Surfaces      map[Surface]FlagSurface
	NewValue      flagValueFactory
	ValueKind     flagValueKind
}

type flagSetDef struct {
	Name      string
	Flags     []*flagDef
	ByName    map[string]int
	ByShort   map[string]int
	Cacheable bool
}

type flagRuntime struct {
	def   *flagDef
	value Value
	isSet bool
}

type cachedFlagDefinition = flagSetDef

func buildCachedFlagDefinition(name string, register func(f *FlagSet)) cachedFlagDefinition {
	return *buildFlagSetDef(name, register)
}

func instantiateCachedFlagDefinition(def cachedFlagDefinition, name string, output io.Writer) (*FlagSet, bool) {
	if !def.Cacheable {
		return nil, false
	}
	return instantiateFlagSetFromDef(&def, name, output), true
}

func buildFlagSetDef(name string, register func(f *FlagSet)) *flagSetDef {
	flagSet := NewFlagSet(name, ContinueOnError)
	flagSet.SetOutput(io.Discard)
	if register != nil {
		register(flagSet)
	}
	return buildFlagSetDefFromFlagSet(flagSet)
}

func buildFlagSetDefFromFlagSet(flagSet *FlagSet) *flagSetDef {
	if flagSet == nil {
		return &flagSetDef{Cacheable: true}
	}

	flags := append([]*Flag(nil), flagSet.formal...)
	sortFlags(flags)

	def := &flagSetDef{
		Name:      flagSet.Name(),
		Flags:     make([]*flagDef, 0, len(flags)),
		ByName:    make(map[string]int, len(flags)),
		ByShort:   make(map[string]int, len(flags)),
		Cacheable: true,
	}

	for _, flag := range flags {
		flagDef, ok := newFlagDefFromFlag(flag)
		if !ok {
			def.Cacheable = false
		}
		index := len(def.Flags)
		def.Flags = append(def.Flags, flagDef)
		def.ByName[flagDef.Name] = index
		if flagDef.Shorthand != "" {
			def.ByShort[flagDef.Shorthand] = index
		}
	}

	return def
}

func instantiateFlagSetFromDef(def *flagSetDef, name string, output io.Writer) *FlagSet {
	flagSet := NewFlagSet(name, ContinueOnError)
	flagSet.SetOutput(output)
	if def == nil {
		return flagSet
	}

	flagSet.def = def
	flagSet.formal = make([]*Flag, len(def.Flags))
	flagSet.isSet = make([]bool, len(def.Flags))
	flagSet.runtime = make([]flagRuntime, len(def.Flags))

	for index, defined := range def.Flags {
		runtime := &flagSet.runtime[index]
		runtime.def = defined
		if defined.NewValue != nil {
			runtime.value = defined.NewValue()
		}
	}

	return flagSet
}

func instantiateFlagView(def *flagDef, runtime *flagRuntime, index int) *Flag {
	flag := &Flag{
		Name:          def.Name,
		Shorthand:     def.Shorthand,
		Usage:         def.Usage,
		Value:         runtime.value,
		DefValue:      def.DefValue,
		ID:            def.ID,
		Kind:          def.Kind,
		Category:      def.Category,
		EnvVars:       append([]string(nil), def.EnvVars...),
		ConfigKeys:    append([]string(nil), def.ConfigKeys...),
		Enum:          append([]string(nil), def.Enum...),
		Required:      def.Required,
		Repeatable:    def.Repeatable,
		Hidden:        def.Hidden,
		Deprecated:    def.Deprecated,
		Example:       def.Example,
		CompletionKey: def.CompletionKey,
		Completion:    def.Completion,
		Extensions:    cloneExtensions(def.Extensions),
		Surfaces:      cloneFlagSurfaces(def.Surfaces),
		index:         index,
		def:           def,
		rt:            runtime,
	}
	return flag
}

func newFlagDefFromFlag(flag *Flag) (*flagDef, bool) {
	if flag == nil {
		return nil, true
	}

	factory, kind, ok := deriveFlagValueFactory(flag)
	if !ok {
		return &flagDef{
			Name:          flag.Name,
			Shorthand:     flag.Shorthand,
			Usage:         flag.Usage,
			DefValue:      flag.DefValue,
			ID:            flag.ID,
			Kind:          flag.Kind,
			Category:      flag.Category,
			EnvVars:       append([]string(nil), flag.EnvVars...),
			ConfigKeys:    append([]string(nil), flag.ConfigKeys...),
			Enum:          append([]string(nil), flag.Enum...),
			Required:      flag.Required,
			Repeatable:    flag.Repeatable,
			Hidden:        flag.Hidden,
			Deprecated:    flag.Deprecated,
			Example:       flag.Example,
			CompletionKey: flag.CompletionKey,
			Completion:    flag.Completion,
			Extensions:    cloneExtensions(flag.Extensions),
			Surfaces:      cloneFlagSurfaces(flag.Surfaces),
			ValueKind:     flagValueKindUnknown,
		}, false
	}

	return &flagDef{
		Name:          flag.Name,
		Shorthand:     flag.Shorthand,
		Usage:         flag.Usage,
		DefValue:      flag.DefValue,
		ID:            flag.ID,
		Kind:          flag.Kind,
		Category:      flag.Category,
		EnvVars:       append([]string(nil), flag.EnvVars...),
		ConfigKeys:    append([]string(nil), flag.ConfigKeys...),
		Enum:          append([]string(nil), flag.Enum...),
		Required:      flag.Required,
		Repeatable:    flag.Repeatable,
		Hidden:        flag.Hidden,
		Deprecated:    flag.Deprecated,
		Example:       flag.Example,
		CompletionKey: flag.CompletionKey,
		Completion:    flag.Completion,
		Extensions:    cloneExtensions(flag.Extensions),
		Surfaces:      cloneFlagSurfaces(flag.Surfaces),
		NewValue:      factory,
		ValueKind:     kind,
	}, true
}

func deriveFlagValueFactory(flag *Flag) (flagValueFactory, flagValueKind, bool) {
	if flag == nil || flag.Value == nil {
		return nil, flagValueKindUnknown, false
	}

	switch value := flag.Value.(type) {
	case *boolValue:
		ptr := (*bool)(value)
		def, err := strconv.ParseBool(flag.DefValue)
		if err != nil {
			return nil, flagValueKindUnknown, false
		}
		return func() Value { return newBoolValue(def, ptr) }, flagValueKindBool, true
	case *intValue:
		ptr := (*int)(value)
		def, err := strconv.Atoi(flag.DefValue)
		if err != nil {
			return nil, flagValueKindUnknown, false
		}
		return func() Value { return newIntValue(def, ptr) }, flagValueKindInt, true
	case *int64Value:
		ptr := (*int64)(value)
		def, err := strconv.ParseInt(flag.DefValue, 0, 64)
		if err != nil {
			return nil, flagValueKindUnknown, false
		}
		return func() Value { return newInt64Value(def, ptr) }, flagValueKindInt64, true
	case *uintValue:
		ptr := (*uint)(value)
		def, err := strconv.ParseUint(flag.DefValue, 0, strconv.IntSize)
		if err != nil {
			return nil, flagValueKindUnknown, false
		}
		return func() Value { return newUintValue(uint(def), ptr) }, flagValueKindUint, true
	case *uint64Value:
		ptr := (*uint64)(value)
		def, err := strconv.ParseUint(flag.DefValue, 0, 64)
		if err != nil {
			return nil, flagValueKindUnknown, false
		}
		return func() Value { return newUint64Value(def, ptr) }, flagValueKindUint64, true
	case *stringValue:
		ptr := (*string)(value)
		def := flag.DefValue
		return func() Value { return newStringValue(def, ptr) }, flagValueKindString, true
	case *float64Value:
		ptr := (*float64)(value)
		def, err := strconv.ParseFloat(flag.DefValue, 64)
		if err != nil {
			return nil, flagValueKindUnknown, false
		}
		return func() Value { return newFloat64Value(def, ptr) }, flagValueKindFloat64, true
	case *durationValue:
		ptr := (*time.Duration)(value)
		def, err := time.ParseDuration(flag.DefValue)
		if err != nil {
			return nil, flagValueKindUnknown, false
		}
		return func() Value { return newDurationValue(def, ptr) }, flagValueKindDuration, true
	case textValue:
		textFactory, ok := deriveTextValueFactory(value)
		if !ok {
			return nil, flagValueKindUnknown, false
		}
		return textFactory, flagValueKindText, true
	case funcValue:
		fn := value
		return func() Value { return fn }, flagValueKindFunc, true
	case boolFuncValue:
		fn := value
		return func() Value { return fn }, flagValueKindBoolFunc, true
	default:
		rv := reflect.ValueOf(value)
		if rv.IsValid() && rv.Kind() == reflect.Ptr && rv.Elem().CanSet() {
			snapshot := reflect.New(rv.Elem().Type())
			snapshot.Elem().Set(rv.Elem())
			return func() Value {
				rv.Elem().Set(snapshot.Elem())
				return value
			}, flagValueKindUnknown, true
		}
		defaultValue := flag.DefValue
		if err := value.Set(defaultValue); err != nil {
			return nil, flagValueKindUnknown, false
		}
		return func() Value {
			_ = value.Set(defaultValue)
			return value
		}, flagValueKindUnknown, true
	}
}

func deriveTextValueFactory(value textValue) (flagValueFactory, bool) {
	ptrVal := reflect.ValueOf(value.p)
	if !ptrVal.IsValid() || ptrVal.Kind() != reflect.Ptr {
		return nil, false
	}
	clone := reflect.New(ptrVal.Elem().Type())
	marshaler, ok := clone.Interface().(encoding.TextMarshaler)
	if !ok {
		return nil, false
	}
	clone.Elem().Set(ptrVal.Elem())
	defaultValue := marshaler
	target := value.p
	return func() Value { return newTextValue(defaultValue, target) }, true
}
