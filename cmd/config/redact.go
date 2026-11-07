package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

const redacted = "[redacted]"

// toTree renders cfg as generic JSON. Unless reveal is set, every value of a
// field tagged secret:"true" that is non-empty is replaced by a placeholder,
// so an agent reading the output never lands a credential in its transcript.
func toTree(cfg any, reveal bool) (any, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var tree any
	if err := json.Unmarshal(b, &tree); err != nil {
		return nil, err
	}
	if !reveal {
		redactTree(tree, reflect.TypeOf(cfg))
	}
	return tree, nil
}

// redactTree walks node alongside the struct type that produced it.
func redactTree(node any, t reflect.Type) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	m, ok := node.(map[string]any)
	if !ok || t.Kind() != reflect.Struct {
		return
	}
	for f := range t.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		child, present := m[name]
		if !present {
			continue
		}
		if f.Tag.Get("secret") == "true" {
			if s, isStr := child.(string); !isStr || s != "" {
				m[name] = redacted
			}
			continue
		}
		redactTree(child, f.Type)
	}
}

// lookup finds a dotted key in tree. Config keys may themselves contain dots
// (evolve.deep-read-cap), so each level tries the longest key that exists.
func lookup(tree any, key string) (any, bool) {
	if key == "" {
		return tree, true
	}
	m, ok := tree.(map[string]any)
	if !ok {
		return nil, false
	}
	for i := len(key); i > 0; i-- {
		if i != len(key) && key[i] != '.' {
			continue
		}
		child, present := m[key[:i]]
		if !present {
			continue
		}
		rest := ""
		if i < len(key) {
			rest = key[i+1:]
		}
		if v, found := lookup(child, rest); found {
			return v, true
		}
	}
	return nil, false
}

func formatValue(v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("format value: %w", err)
	}
	return string(b), nil
}
