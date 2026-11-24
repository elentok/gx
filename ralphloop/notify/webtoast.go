package notify

// WebToastNote is one notification handed to the Web toast channel. The
// adapter applies the channel switch and kind list; callers send every one.
type WebToastNote struct {
	Kind        string // catalog id
	Title, Body string
	Target      Target
}

// WebToast is the channel that shows a toast on every open page.
type WebToast interface{ Show(WebToastNote) }

// WebToast hands one note to the Web toast channel. A nil chat or channel sends none.
func (c *ServerChat) WebToast(n WebToastNote) {
	if c == nil || c.base.WebToast == nil {
		return
	}
	c.base.WebToast.Show(n)
}
