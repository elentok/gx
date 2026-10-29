package server

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"io"
	"testing"
)

// plistDicts returns each dict's key to value text, in document order.
func plistDicts(t *testing.T, data []byte) []map[string]string {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(data))
	var dicts []map[string]string
	var stack []map[string]string
	var key string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return dicts
		}
		if err != nil {
			t.Fatalf("plist does not parse: %v", err)
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "dict":
				m := map[string]string{}
				dicts = append(dicts, m)
				stack = append(stack, m)
			case "key":
				var k string
				_ = dec.DecodeElement(&k, &e)
				key = k
			case "string", "data", "integer":
				var v string
				_ = dec.DecodeElement(&v, &e)
				stack[len(stack)-1][key] = v
			}
		case xml.EndElement:
			if e.Name.Local == "dict" {
				stack = stack[:len(stack)-1]
			}
		}
	}
}

func TestWebRootProfileHasOneRootPayload(t *testing.T) {
	der := []byte("root der bytes")
	data, err := webRootProfile(der)
	if err != nil {
		t.Fatal(err)
	}
	dicts := plistDicts(t, data)
	if len(dicts) != 2 {
		t.Fatalf("%d dicts, want the profile and one payload", len(dicts))
	}
	top, payload := dicts[0], dicts[1]
	if top["PayloadType"] != "Configuration" || payload["PayloadType"] != "com.apple.security.root" {
		t.Errorf("types %q / %q", top["PayloadType"], payload["PayloadType"])
	}
	got, err := base64.StdEncoding.DecodeString(payload["PayloadContent"])
	if err != nil || !bytes.Equal(got, der) {
		t.Errorf("payload data %q (%v), want the root DER", got, err)
	}
}

func TestWebRootProfileIdentifierIsStableAndUUIDIsFresh(t *testing.T) {
	der := []byte("root der bytes")
	a, _ := webRootProfile(der)
	b, _ := webRootProfile(der)
	da, db := plistDicts(t, a)[0], plistDicts(t, b)[0]
	if da["PayloadIdentifier"] == "" || da["PayloadIdentifier"] != db["PayloadIdentifier"] {
		t.Errorf("identifiers %q / %q, want equal and set", da["PayloadIdentifier"], db["PayloadIdentifier"])
	}
	if da["PayloadUUID"] == "" || da["PayloadUUID"] == db["PayloadUUID"] {
		t.Errorf("UUIDs %q / %q, want different", da["PayloadUUID"], db["PayloadUUID"])
	}
}
