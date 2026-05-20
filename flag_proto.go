package cmd

import (
	"io"
	"strconv"
	"time"
)

type flagValueFactory func() Value

type cachedFlagDefinition struct {
	flags     []cachedFlagRecord
	sorted    bool
	cacheable bool
}

type cachedFlagRecord struct {
	flag    *Flag
	factory flagValueFactory
}

func buildCachedFlagDefinition(name string, register func(f *FlagSet)) cachedFlagDefinition {
	if register == nil {
		return cachedFlagDefinition{}
	}

	flagSet := NewFlagSet(name, ContinueOnError)
	flagSet.SetOutput(io.Discard)
	register(flagSet)

	records := make([]cachedFlagRecord, 0, len(flagSet.formal))
	cacheable := true
	for _, flag := range flagSet.formal {
		factory, ok := deriveFlagValueFactory(flag)
		if !ok {
			cacheable = false
		}
		records = append(records, cachedFlagRecord{
			flag:    cloneFlag(flag),
			factory: factory,
		})
	}

	return cachedFlagDefinition{
		flags:     records,
		sorted:    flagSet.sorted,
		cacheable: cacheable,
	}
}

func instantiateCachedFlagDefinition(def cachedFlagDefinition, name string, output io.Writer) (*FlagSet, bool) {
	if !def.cacheable {
		return nil, false
	}

	flagSet := NewFlagSet(name, ContinueOnError)
	flagSet.SetOutput(output)
	flagSet.formal = make([]*Flag, 0, len(def.flags))
	for _, record := range def.flags {
		cloned := cloneFlag(record.flag)
		if record.factory != nil {
			cloned.Value = record.factory()
		}
		flagSet.formal = append(flagSet.formal, cloned)
	}
	flagSet.sorted = def.sorted
	return flagSet, true
}

func deriveFlagValueFactory(flag *Flag) (flagValueFactory, bool) {
	if flag == nil || flag.Value == nil {
		return nil, false
	}

	switch value := flag.Value.(type) {
	case *boolValue:
		ptr := (*bool)(value)
		def, err := strconv.ParseBool(flag.DefValue)
		if err != nil {
			return nil, false
		}
		return func() Value { return newBoolValue(def, ptr) }, true
	case *intValue:
		ptr := (*int)(value)
		def, err := strconv.Atoi(flag.DefValue)
		if err != nil {
			return nil, false
		}
		return func() Value { return newIntValue(def, ptr) }, true
	case *int64Value:
		ptr := (*int64)(value)
		def, err := strconv.ParseInt(flag.DefValue, 0, 64)
		if err != nil {
			return nil, false
		}
		return func() Value { return newInt64Value(def, ptr) }, true
	case *uintValue:
		ptr := (*uint)(value)
		def, err := strconv.ParseUint(flag.DefValue, 0, strconv.IntSize)
		if err != nil {
			return nil, false
		}
		return func() Value { return newUintValue(uint(def), ptr) }, true
	case *uint64Value:
		ptr := (*uint64)(value)
		def, err := strconv.ParseUint(flag.DefValue, 0, 64)
		if err != nil {
			return nil, false
		}
		return func() Value { return newUint64Value(def, ptr) }, true
	case *stringValue:
		ptr := (*string)(value)
		def := flag.DefValue
		return func() Value { return newStringValue(def, ptr) }, true
	case *float64Value:
		ptr := (*float64)(value)
		def, err := strconv.ParseFloat(flag.DefValue, 64)
		if err != nil {
			return nil, false
		}
		return func() Value { return newFloat64Value(def, ptr) }, true
	case *durationValue:
		ptr := (*time.Duration)(value)
		def, err := time.ParseDuration(flag.DefValue)
		if err != nil {
			return nil, false
		}
		return func() Value { return newDurationValue(def, ptr) }, true
	case funcValue:
		fn := value
		return func() Value { return fn }, true
	case boolFuncValue:
		fn := value
		return func() Value { return fn }, true
	default:
		return nil, false
	}
}
