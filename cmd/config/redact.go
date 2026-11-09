package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const redacted = "[redacted]"

// toTree renders cfg as generic JSON. Unless reveal is set, every value of a
// field tagged secret:"true" that is non-empty is replaced by a placeholder,
// so an agent reading the output never lands a credential in its transcript.
// The second result lists the dotted paths that were redacted (slice elements
// by index); it is empty when reveal is set.
func toTree(cfg any, reveal bool) (any, []string, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, nil, err
	}
	var tree any
	if err := json.Unmarshal(b, &tree); err != nil {
		return nil, nil, err
	}
	var paths []string
	if !reveal {
		redactTree(tree, reflect.TypeOf(cfg), "", &paths)
	}
	return tree, paths, nil
}

// touchesSecret reports whether key is, sits inside, or contains one of the
// redacted paths.
func touchesSecret(key string, paths []string) bool {
	for _, p := range paths {
		if key == "" || key == p || strings.HasPrefix(key, p+".") || strings.HasPrefix(p, key+".") {
			return true
		}
	}
	return false
}

func joinPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

// redactTree walks node alongside the type that produced it, descending
// structs, slices, arrays and maps, and records each redacted path in out.
func redactTree(node any, t reflect.Type, path string, out *[]string) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		items, _ := node.([]any)
		for i, item := range items {
			redactTree(item, t.Elem(), joinPath(path, strconv.Itoa(i)), out)
		}
	case reflect.Map:
		m, _ := node.(map[string]any)
		for k, item := range m {
			redactTree(item, t.Elem(), joinPath(path, k), out)
		}
	case reflect.Struct:
		m, ok := node.(map[string]any)
		if !ok {
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
					*out = append(*out, joinPath(path, name))
				}
				continue
			}
			redactTree(child, f.Type, joinPath(path, name), out)
		}
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
