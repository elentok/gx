package ui

import "reflect"

// Memo remembers the last render and its key, so a View whose inputs did not
// change skips the string-width measuring a render pays. A pointer, so every
// copy of a Bubble Tea model shares it; a nil Memo renders every time.
type Memo struct {
	key any
	out string
	ok  bool
}

// Get returns the last output when key deep-equals the last key, else render()'s.
func (c *Memo) Get(key any, render func() string) string {
	if c == nil {
		return render()
	}
	if c.ok && reflect.DeepEqual(c.key, key) {
		return c.out
	}
	c.key, c.out, c.ok = key, render(), true
	return c.out
}
