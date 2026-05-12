# Keybinding manager

We currently have a lot of messy keybinding handling code:

- long switch statements with custom handling for chords
- duplication of the keys for the help page

I want to design a "Keybinding" component/model where I can register the keybindings,
it should make it easier to register new bindings, should handle chords.

I have a few concept ideas:

```go
package keybindings

type Keybindings struct {
  keybinding []Keybinding
}

func New(keybinding[]Keybinding) Keybindings {
  return Keybindings{
    keybinding: keybinding,
  }
}

func (k *Keybindings) HelpView() string { ... }

type Keybinding struct {
  id int
  category string
  title string
  keys []string // for chords
}

---

type Key int

const (
  SEARCH Key = iota,
  GO_UP,
  GO_DOWN,
  GO_LEFT,
  GO_RIGHT,
  // ...
)

globalKeybindings := keybindings.New(Keybinding[]{
  Keybinding{GO_TO_STATUS, "Nav", "go to status", "s", prefixKey: "g"},
  Keybinding{GO_TO_LOG, "Nav", "go to log", "l", prefixKey: "g"},
})

statusKeybindings := keybindings.New(Keybinding[]{
  Keybinding{PREV_FILE, "prev file", ","},
  Keybinding{NEXT_FILE, "next file", "."},
})
```

I'm not 100% sure about the design, I want it to be idiomatic go.

wdyt?
