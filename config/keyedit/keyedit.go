// Package keyedit edits a JSON object document by key path while keeping the
// order of every object's keys, which a decode-into-map and re-marshal does not.
package keyedit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// object is an ordered JSON object. A value is either a nested *object or a
// json.RawMessage kept byte for byte.
type object struct {
	keys []string
	vals map[string]any
}

func newObject() *object { return &object{vals: map[string]any{}} }

func (o *object) set(key string, v any) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = v
}

func (o *object) remove(key string) {
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

func parseObject(raw []byte) (*object, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil {
		return nil, err
	} else if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("not a JSON object")
	}
	obj := newObject()
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key := tok.(string)
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, err
		}
		if len(val) > 0 && val[0] == '{' {
			child, err := parseObject(val)
			if err != nil {
				return nil, err
			}
			obj.set(key, child)
		} else {
			obj.set(key, val)
		}
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected data after top-level value")
	}
	return obj, nil
}

func (o *object) write(buf *bytes.Buffer) {
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		switch v := o.vals[k].(type) {
		case *object:
			v.write(buf)
		case json.RawMessage:
			buf.Write(v)
		}
	}
	buf.WriteByte('}')
}

func load(doc []byte) (*object, error) {
	if len(bytes.TrimSpace(doc)) == 0 {
		return newObject(), nil
	}
	return parseObject(doc)
}

func render(root *object) ([]byte, error) {
	var compact bytes.Buffer
	root.write(&compact)
	var out bytes.Buffer
	if err := json.Indent(&out, compact.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

func pathErr(path []string, err error) error {
	return fmt.Errorf("keyedit %q: %w", strings.Join(path, "."), err)
}

// Set sets or replaces the key at path. An existing key keeps its place, a new
// key is appended to its object, and missing parent objects are created.
func Set(doc []byte, path []string, value any) ([]byte, error) {
	if len(path) == 0 {
		return nil, pathErr(path, fmt.Errorf("empty path"))
	}
	root, err := load(doc)
	if err != nil {
		return nil, pathErr(path, err)
	}
	var vb bytes.Buffer
	enc := json.NewEncoder(&vb)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, pathErr(path, err)
	}
	raw := json.RawMessage(bytes.TrimSpace(vb.Bytes()))
	var leaf any = raw
	if len(raw) > 0 && raw[0] == '{' {
		if leaf, err = parseObject(raw); err != nil {
			return nil, pathErr(path, err)
		}
	}

	cur := root
	for i, seg := range path[:len(path)-1] {
		switch next := cur.vals[seg].(type) {
		case *object:
			cur = next
		case nil:
			child := newObject()
			cur.set(seg, child)
			cur = child
		default:
			return nil, pathErr(path[:i+1], fmt.Errorf("parent is not an object"))
		}
	}
	cur.set(path[len(path)-1], leaf)
	out, err := render(root)
	if err != nil {
		return nil, pathErr(path, err)
	}
	return out, nil
}

// Remove removes the key at path. A nested object left empty is removed too;
// an absent key is a no-op.
func Remove(doc []byte, path []string) ([]byte, error) {
	if len(path) == 0 {
		return nil, pathErr(path, fmt.Errorf("empty path"))
	}
	root, err := load(doc)
	if err != nil {
		return nil, pathErr(path, err)
	}
	chain := []*object{root}
	cur := root
	for i, seg := range path[:len(path)-1] {
		switch next := cur.vals[seg].(type) {
		case *object:
			cur = next
			chain = append(chain, cur)
		case nil:
			return render(root)
		default:
			return nil, pathErr(path[:i+1], fmt.Errorf("parent is not an object"))
		}
	}
	cur.remove(path[len(path)-1])
	for i := len(chain) - 1; i > 0 && len(chain[i].keys) == 0; i-- {
		chain[i-1].remove(path[i-1])
	}
	out, err := render(root)
	if err != nil {
		return nil, pathErr(path, err)
	}
	return out, nil
}
