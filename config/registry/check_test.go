package registry

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCheckValue(t *testing.T) {
	min := 1.0
	parseSize := func(s string) (int64, error) {
		if s == "20G" {
			return 20, nil
		}
		return 0, errors.New("bad")
	}
	tests := []struct {
		name  string
		entry Entry
		raw   string
		ok    bool
	}{
		{"bool ok", Entry{Type: TypeBool}, `true`, true},
		{"bool bad", Entry{Type: TypeBool}, `"true"`, false},
		{"number ok", Entry{Type: TypeNumber, Min: &min}, `3`, true},
		{"number below min", Entry{Type: TypeNumber, Min: &min}, `0`, false},
		{"number not integer", Entry{Type: TypeNumber, Integer: true}, `1.5`, false},
		{"number string", Entry{Type: TypeNumber}, `"3"`, false},
		{"duration ok", Entry{Type: TypeDuration}, `"5m"`, true},
		{"duration 5mm", Entry{Type: TypeDuration}, `"5mm"`, false},
		{"duration zero", Entry{Type: TypeDuration}, `"0s"`, false},
		{"duration negative", Entry{Type: TypeDuration}, `"-5m"`, false},
		{"duration number", Entry{Type: TypeDuration}, `5`, false},
		{"size ok", Entry{Type: TypeSize}, `"20G"`, true},
		{"size bad", Entry{Type: TypeSize}, `"5x"`, false},
		{"enum ok", Entry{Type: TypeEnum, Enum: []string{"a", "b"}}, `"a"`, true},
		{"enum non-member", Entry{Type: TypeEnum, Enum: []string{"a", "b"}}, `"c"`, false},
		{"string ok", Entry{Type: TypeString}, `"x"`, true},
		{"string bad", Entry{Type: TypeString}, `1`, false},
		{"string list ok", Entry{Type: TypeStringList}, `["a"]`, true},
		{"string list bad", Entry{Type: TypeStringList}, `["a", 1]`, false},
		{"number list ok", Entry{Type: TypeNumberList}, `[1, 2]`, true},
		{"number list string", Entry{Type: TypeNumberList}, `[1, "abc"]`, false},
		{"secret ok", Entry{Type: TypeSecret}, `"s"`, true},
		{"secret bad", Entry{Type: TypeSecret}, `true`, false},
		{"checks ok", Entry{Type: TypeChecks}, `{"a": []}`, true},
		{"checks bad", Entry{Type: TypeChecks}, `[]`, false},
		{"null", Entry{Type: TypeString}, `null`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := json.RawMessage(tt.raw)
			err := CheckValue(tt.entry, raw, parseSize)
			if tt.ok && err != nil {
				t.Fatalf("want nil, got %v", err)
			}
			if !tt.ok && (err == nil || strings.TrimSpace(err.Error()) == "") {
				t.Fatalf("want a reason, got %v", err)
			}
			if string(raw) != tt.raw {
				t.Fatalf("value changed to %s", raw)
			}
		})
	}
}
