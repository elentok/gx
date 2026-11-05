// Package prompt holds what a person sends into a session: the first prompt,
// prompts and keys, session actions, auto-advance and turn settling.
//
// It sits above sessions and implements the Prompter hook that sessions
// defines; sessions never imports it.
package prompt
