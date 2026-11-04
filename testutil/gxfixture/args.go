package main

import "fmt"

// opts is what the command line asked for, before the scenario is looked up.
type opts struct {
	gx, fakeClaude, fakeCodex, scenario         string
	autoAdvance, commandListAge, stallThreshold string
	mockDir, shape                              string
	configs                                     []string
	hold, shellEnv, hideCodex                   bool
}

// parseArgs reads the flags and the positional scenario name. It does not look
// the scenario up, so it needs no registry and touches nothing.
func parseArgs(args []string) (opts, error) {
	var o opts
	bools := map[string]*bool{"--hide-codex": &o.hideCodex, "--hold": &o.hold, "--env": &o.shellEnv}
	values := map[string]*string{
		"--auto-advance-delay": &o.autoAdvance,
		"--command-list-age":   &o.commandListAge,
		"--stall-threshold":    &o.stallThreshold,
		"--mock-dir":           &o.mockDir,
		"--shape":              &o.shape,
		"--gx":                 &o.gx,
		"--fakeclaude":         &o.fakeClaude,
		"--fakecodex":          &o.fakeCodex,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if b, ok := bools[arg]; ok {
			*b = true
			continue
		}
		dst, isValue := values[arg]
		if !isValue && arg != "--config" {
			o.scenario = arg
			continue
		}
		if i+1 == len(args) {
			return opts{}, fmt.Errorf("%s needs a value", arg)
		}
		i++
		if isValue {
			*dst = args[i]
		} else {
			o.configs = append(o.configs, args[i])
		}
	}
	return o, nil
}
