package rest

import "strings"

type messagePair struct{ message, sub string }

// builtinBreakMessages is the spec's list; {work} is the Work time.
var builtinBreakMessages = []messagePair{
	{"{work} minutes in a row? The agents will survive without you.", "Stand up, drink some water, look at something far away."},
	{"Your chair called. It wants a day off from you.", "Walk to another room and come back slower."},
	{"Even the agents get to idle sometimes.", "You can too. Stretch your back and your wrists."},
	{"{work} minutes of focus. The mascot is proud and slightly worried.", "Look out of a window for a while."},
	{"Nothing here is on fire. Probably.", "The Queue is working. Go get a glass of water."},
	{"Your eyes have been staring at a screen for {work} minutes.", "Look at something 6 metres away for 20 seconds."},
	{"The code will still be here after a short walk.", "Roll your shoulders and unclench your jaw."},
	{"Breaks are not a bug. They are a feature.", "Close your eyes and take five slow breaths."},
	{"Hydrate or diedrate.", "Fill a glass and drink it before you come back."},
	{"You have been very productive. Do not ruin it by burning out.", "Stand up and stretch for a minute."},
	{"The agents ask you to please stop watching them.", "They work better when nobody stares."},
	{"Time for a tiny holiday.", "Go outside if you can, or just breathe by the window."},
}

// splitMessage cuts text at its first sentence end (., ! or ? followed by a
// space): the first sentence is the message, the rest the sub-message.
func splitMessage(text string) (message, sub string) {
	text = strings.TrimSpace(text)
	for i := 0; i+1 < len(text); i++ {
		if text[i+1] == ' ' && strings.IndexByte(".!?", text[i]) >= 0 {
			return text[:i+1], strings.TrimSpace(text[i+2:])
		}
	}
	return text, ""
}
