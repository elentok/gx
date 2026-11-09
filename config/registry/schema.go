package registry

import (
	"bytes"
	"encoding/json"
)

// Defaults reports a key's default value for the generated config schema. The
// registry holds none, so the caller supplies them.
type Defaults func(KeyPath) (any, bool)

type schemaNode struct {
	leaf     map[string]any // set on a leaf
	children map[string]*schemaNode
}

func newSchemaNode() *schemaNode { return &schemaNode{children: map[string]*schemaNode{}} }

// set puts a leaf at path unless the path, or an ancestor, is already taken.
func (n *schemaNode) set(path KeyPath, leaf map[string]any) {
	for _, seg := range path[:len(path)-1] {
		child, ok := n.children[seg]
		if !ok {
			child = newSchemaNode()
			n.children[seg] = child
		}
		if child.leaf != nil {
			return
		}
		n = child
	}
	last := path[len(path)-1]
	if _, taken := n.children[last]; !taken {
		n.children[last] = &schemaNode{leaf: leaf}
	}
}

func (n *schemaNode) render() map[string]any {
	if n.leaf != nil {
		return n.leaf
	}
	props := map[string]any{}
	for name, child := range n.children {
		props[name] = child.render()
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": props}
}

// entrySchema is the JSON Schema of one entry's value.
func entrySchema(e Entry) map[string]any {
	s := map[string]any{"description": e.Description}
	switch e.Type {
	case TypeBool:
		s["type"] = "boolean"
	case TypeNumber:
		s["type"] = "number"
		if e.Integer {
			s["type"] = "integer"
		}
		if e.Min != nil {
			s["minimum"] = *e.Min
		}
	case TypeNumberList:
		s["type"] = "array"
		s["items"] = map[string]any{"type": "number", "minimum": 0}
	case TypeStringList:
		s["type"] = "array"
		s["items"] = map[string]any{"type": "string"}
	case TypeEnum:
		s["type"] = "string"
		s["enum"] = e.Enum
	case TypeChecks:
		s["$ref"] = "checks-schema.json"
	default: // duration, size, string, secret
		s["type"] = "string"
	}
	return s
}

const (
	configSchemaID  = "https://raw.githubusercontent.com/elentok/gx/main/docs/config-schema.json"
	projectSchemaID = "https://raw.githubusercontent.com/elentok/gx/main/docs/project-schema.json"
)

// ConfigSchema renders docs/config-schema.json. Excluded keys stay valid with
// no type constraint, so files that hold them still validate in an editor.
func (r *Registry) ConfigSchema(defaults Defaults) []byte {
	root := newSchemaNode()
	for _, e := range r.entries {
		if e.ProjectOnly {
			continue
		}
		s := entrySchema(e)
		if v, ok := defaults(e.Key); ok {
			s["default"] = v
		}
		root.set(e.Key, s)
	}
	for _, x := range Exclusions() {
		if !x.Project {
			root.set(x.Key, map[string]any{"description": x.Reason})
		}
	}
	return render(root, configSchemaID, "gx config", "")
}

// ProjectSchema renders docs/project-schema.json. An excluded top-level key
// reuses its config.json definition.
func (r *Registry) ProjectSchema() []byte {
	root := newSchemaNode()
	for _, e := range r.ForProject() {
		root.set(e.ProjectPath(), entrySchema(e))
	}
	for _, x := range Exclusions() {
		if !x.Project {
			continue
		}
		leaf := map[string]any{"description": x.Reason}
		if len(x.Key) == 1 {
			leaf["$ref"] = "config-schema.json#/properties/" + x.Key[0]
		}
		root.set(x.Key, leaf)
	}
	return render(root, projectSchemaID, "gx project",
		"A project's project.json in the ticket store. name and repo identify the project; the other keys override config.json for this project only.")
}

func render(root *schemaNode, id, title, description string) []byte {
	doc := root.render()
	doc["$schema"] = "http://json-schema.org/draft-07/schema#"
	doc["$id"] = id
	doc["title"] = title
	if description != "" {
		doc["description"] = description
	}
	doc["properties"].(map[string]any)["$schema"] = map[string]any{"type": "string"}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		panic(err) // the document is built from plain maps and slices
	}
	return buf.Bytes()
}
