package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// Checks reads and saves a project's check configuration.
type Checks struct {
	cfg      func() projectsEnv
	projects *Registry
	guards   *runs.Guards
}

func (s *Server) checksStatusRead(w http.ResponseWriter, r *http.Request) {
	st, err := s.checks.Status(r.URL.Query().Get("project"))
	if err != nil {
		code := http.StatusInternalServerError
		var ae *tickets.AddressError
		if errors.As(err, &ae) {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	wire.WriteJSON(w, st)
}

// checksStatus answers for one project, or for every registered project when
// project is empty.
func (c *Checks) Status(project string) (wire.ChecksStatus, error) {
	st := wire.ChecksStatus{Projects: []wire.ProjectChecks{}}
	var dirs []string
	if project != "" {
		dir, err := tickets.ProjectDirByName(c.cfg().TicketStore, project)
		if err != nil {
			return st, err
		}
		dirs = []string{dir}
	} else {
		var err error
		if dirs, err = tickets.ProjectDirs(c.cfg().TicketStore); err != nil {
			return st, err
		}
	}
	for _, dir := range dirs {
		st.Projects = append(st.Projects, c.Project(dir))
	}
	return st, nil
}

func (c *Checks) Project(dir string) wire.ProjectChecks {
	name := tickets.ProjectName(dir)
	pf, err := config.ReadProjectFile(dir)
	pc := wire.ProjectChecks{Project: name, SaveTargets: []string{wire.SaveTargetRepo, wire.SaveTargetProjectJSON}, Kinds: []wire.KindCheck{}}
	if err != nil {
		pc.Error = err.Error()
	}
	vcsNone := pf.VCS != nil && *pf.VCS == config.VCSNone
	if vcsNone {
		pc.SaveTargets = []string{wire.SaveTargetProjectJSON}
	}
	root := tickets.TrunkCheckout(c.cfg().TicketStore, name)
	if repo := tickets.ProjectRepoRoot(c.cfg().TicketStore, name); repo != "" {
		pc.Trunk, _ = tickets.Trunk(dir, repo)
	}

	cfg, hasConfig, cfgErr := loadChecks(root, pf)
	if cfgErr != nil {
		pc.Error = cfgErr.Error()
	}
	// The built-in scratch project, and the gx-made repo-less outbox, have no
	// code to check.
	builtIn := name == ScratchProject || (name == OutboxProject && vcsNone)
	pc.Missing = !hasConfig && !builtIn && pc.Error == ""

	var found map[string][]check.Candidate
	if root != "" {
		found = check.Detect(root)
	}
	for _, kind := range check.Kinds {
		kc := wire.KindCheck{Kind: kind, State: wire.KindStateDefault, Default: check.DefaultLine[kind], Candidates: []check.Candidate{}}
		if c, ok := cfg[kind]; ok {
			kc.State = wire.KindStateConfigured
			if c.Off {
				kc.State = wire.KindStateOff
			}
		}
		if cands := found[kind]; len(cands) > 0 {
			kc.Recommended = &cands[0]
			kc.Candidates = cands[1:]
		}
		pc.Kinds = append(pc.Kinds, kc)
	}
	return pc
}

// loadChecks reads the repo file in root, else the project.json checks key.
// hasConfig is true when either exists, even when it is invalid or empty.
func loadChecks(root string, pf config.ProjectFile) (cfg check.Config, hasConfig bool, err error) {
	hasFile := false
	if root != "" {
		_, statErr := os.Stat(filepath.Join(root, check.RepoConfigPath))
		hasFile = statErr == nil
	}
	hasConfig = hasFile || len(pf.Checks) > 0
	if !hasConfig {
		return nil, false, nil
	}
	if hasFile {
		cfg, err = check.LoadConfig(root, nil)
	} else {
		cfg, err = check.ParseConfig(pf.Checks)
		if err != nil {
			err = errors.New("project.json checks: " + err.Error())
		}
	}
	return cfg, true, err
}
