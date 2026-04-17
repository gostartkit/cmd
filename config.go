package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type ConfigLoader func(path string) (map[string]any, error)

type ConfigFlagOptions struct {
	Name      string
	Shorthand string
	Usage     string
	EnvVars   []string
	Example   string
}

func LoadJSONConfig(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var data map[string]any
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	return data, nil
}

func defaultConfigEnvVar(appName string) string {
	var b strings.Builder
	for _, r := range appName {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - ('a' - 'A'))
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "APP_CONFIG"
	}
	b.WriteString("_CONFIG")
	return b.String()
}

func (a *App) loadConfigData() error {
	a.configData = nil
	if !a.configEnabled() || a.flag == nil {
		return nil
	}
	configFlag, ok := a.flag.Lookup(a.ConfigFlag.Name)
	if !ok {
		return nil
	}
	configPath := configFlag.Value.String()
	if configPath == "" {
		return nil
	}
	if a.ConfigLoader == nil {
		a.ConfigLoader = LoadJSONConfig
	}
	data, err := a.ConfigLoader(configPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", configPath, err)
	}
	a.configData = data
	return nil
}

func configValueAtPath(data map[string]any, path string) (any, bool) {
	if len(data) == 0 || path == "" {
		return nil, false
	}

	current := any(data)
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := object[part]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

func stringifyConfigValue(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case bool:
		if typed {
			return "true", nil
		}
		return "false", nil
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.12f", typed), "0"), "."), nil
	case json.Number:
		return typed.String(), nil
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", typed), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", typed), nil
	default:
		return "", fmt.Errorf("unsupported config value type %T", value)
	}
}
