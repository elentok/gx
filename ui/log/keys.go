package log

import "github.com/elentok/gx/ui/keys"

var (
	logKeySearch     = keys.Binding{Seq: []string{"/"}, Title: "search"}
	logKeyResultNext = keys.Binding{Seq: []string{"n"}, Title: "next result"}
	logKeyResultPrev = keys.Binding{Seq: []string{"N"}, Title: "prev result"}
)
