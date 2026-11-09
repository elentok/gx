package keyedit

import (
	"strings"
	"testing"
)

func TestSet(t *testing.T) {
	tests := []struct {
		name  string
		doc   string
		path  []string
		value any
		want  string
	}{
		{"replace keeps place", `{"b":1,"a":2,"c":3}`, []string{"a"}, 9, "{\n  \"b\": 1,\n  \"a\": 9,\n  \"c\": 3\n}\n"},
		{"new key appended last", `{"b":1,"a":2}`, []string{"z"}, "x", "{\n  \"b\": 1,\n  \"a\": 2,\n  \"z\": \"x\"\n}\n"},
		{"parent created", `{"a":1}`, []string{"p", "q"}, true, "{\n  \"a\": 1,\n  \"p\": {\n    \"q\": true\n  }\n}\n"},
		{"empty array kept", `{}`, []string{"cleanup-command"}, []string{}, "{\n  \"cleanup-command\": []\n}\n"},
		{"unknown keys and nested order kept", `{"z":{"y":1,"x":[1,2]},"unknown":null}`, []string{"z", "w"}, 3,
			"{\n  \"z\": {\n    \"y\": 1,\n    \"x\": [\n      1,\n      2\n    ],\n    \"w\": 3\n  },\n  \"unknown\": null\n}\n"},
		{"dotted key is one key", `{"evolve":{"a":1}}`, []string{"evolve.deep-read-cap"}, 5, "{\n  \"evolve\": {\n    \"a\": 1\n  },\n  \"evolve.deep-read-cap\": 5\n}\n"},
		{"empty doc", ``, []string{"a"}, 1, "{\n  \"a\": 1\n}\n"},
		{"no html escaping", `{}`, []string{"a"}, "<b>", "{\n  \"a\": \"<b>\"\n}\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Set([]byte(tc.doc), tc.path, tc.value)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestRemove(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		path []string
		want string
	}{
		{"removes key keeps order", `{"b":1,"a":2,"c":3}`, []string{"a"}, "{\n  \"b\": 1,\n  \"c\": 3\n}\n"},
		{"empty parent removed", `{"x":1,"p":{"q":1}}`, []string{"p", "q"}, "{\n  \"x\": 1\n}\n"},
		{"empty chain removed", `{"x":1,"p":{"m":{"q":1}}}`, []string{"p", "m", "q"}, "{\n  \"x\": 1\n}\n"},
		{"non-empty parent stays", `{"p":{"q":1,"r":2}}`, []string{"p", "q"}, "{\n  \"p\": {\n    \"r\": 2\n  }\n}\n"},
		{"absent key no-op", `{"a":1}`, []string{"nope"}, "{\n  \"a\": 1\n}\n"},
		{"absent parent no-op", `{"a":1}`, []string{"nope", "x"}, "{\n  \"a\": 1\n}\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Remove([]byte(tc.doc), tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	if _, err := Set([]byte(`{"a":`), []string{"a", "b"}, 1); err == nil || !strings.Contains(err.Error(), "a.b") {
		t.Fatalf("malformed: %v", err)
	}
	if _, err := Set([]byte(`[1]`), []string{"a"}, 1); err == nil {
		t.Fatal("array root should error")
	}
	if _, err := Set([]byte(`{"a":5}`), []string{"a", "b"}, 1); err == nil || !strings.Contains(err.Error(), "a") {
		t.Fatalf("non-object parent: %v", err)
	}
	if _, err := Remove([]byte(`{"a":5}`), []string{"a", "b"}); err == nil {
		t.Fatal("remove under non-object should error")
	}
	if _, err := Remove([]byte(`{`), []string{"a"}); err == nil {
		t.Fatal("malformed remove should error")
	}
}
