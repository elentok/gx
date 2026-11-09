package check

import (
	"encoding/json"
)

// obj is one JSON object of the generated schema; encoding/json sorts its keys,
// so the output is stable.
type obj = map[string]any

// SchemaJSON renders docs/checks-schema.json from Kinds and the command and
// limits shape that parseCommand and parseLimits accept.
func SchemaJSON() []byte {
	ref := func(name string) obj { return obj{"$ref": "#/definitions/" + name} }
	str := obj{"type": "string", "minLength": 1}

	props := obj{"$schema": obj{"type": "string"}}
	for _, kind := range Kinds {
		props[kind] = ref("check")
	}

	tools := obj{}
	for tool := range slowToolAliases {
		tools[tool] = ref("duration")
	}

	limits := obj{
		"type":                 "object",
		"additionalProperties": false,
		"description":          "Time limits for a test entry. A missing key turns that limit off.",
		"properties": obj{
			"suite": ref("duration"),
			"hang":  ref("duration"),
			"slow": obj{
				"description": "Per-test slow limit: one duration for every tool, or a map by tool.",
				"oneOf": []any{
					ref("duration"),
					obj{"type": "object", "additionalProperties": false, "properties": tools},
				},
			},
		},
	}

	pair := obj{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"full"},
		"properties": obj{
			"changed": obj{
				"type": "string", "minLength": 1,
				"description": "Command for a changed-files run. Absent: the gx default.",
			},
			"full": obj{
				"type": "string", "minLength": 1,
				"description": "Command for a --full run, with {files} set to every tracked file.",
			},
			"always": obj{
				"type":        "array",
				"items":       str,
				"description": "Commands that also run in every non-full run (changed, staged, --since), through the same path as the scoped command. A --full run covers them already.",
			},
			"limits": limits,
		},
	}

	schema := obj{
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"$id":                  "https://raw.githubusercontent.com/elentok/gx/main/docs/checks-schema.json",
		"title":                "gx checks",
		"description":          "A project's .gx/checks.json (or the checks key of project.json): the commands `gx check` runs. {files} in a command is replaced by the selected files.",
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
		"definitions": obj{
			"duration": obj{
				"type":        "string",
				"description": "A positive Go duration such as \"20s\" or \"4m\".",
				"minLength":   1,
			},
			"check": obj{
				"description": "A command for both scopes, false to turn the kind off, or a pair of commands.",
				"oneOf":       []any{str, obj{"const": false}, pair},
			},
		},
	}

	b, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		panic(err) // the schema is built from literals; it always marshals
	}
	return append(b, '\n')
}
