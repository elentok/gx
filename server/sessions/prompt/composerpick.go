package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

// composerPicker is the Draft history half of Prompt, split out like imageStore.
type composerPicker struct{ *imageStore }

func composerRefusal(reason, msg string) ops.ComposerPickResult {
	return ops.ComposerPickResult{Refused: true, Reason: reason, Message: msg, Images: []string{}, Notices: []string{}}
}

// composerPickHandler serves POST /v1/sessions/pick-history.
func (p composerPicker) composerPickHandler(w http.ResponseWriter, r *http.Request) {
	var req ops.ComposerPickRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := p.pickComposer(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

// pickComposer replaces the composer's text and images with a Sent messages
// entry's. The composer's own text and images go to Sent messages first. Only
// the text and the images change: the session's meta is not touched.
func (p composerPicker) pickComposer(req ops.ComposerPickRequest) (ops.ComposerPickResult, error) {
	_, meta, sres, err := p.sessions.SessionDir(req.SessionID)
	if err != nil {
		return ops.ComposerPickResult{}, err
	}
	if sres.Refused {
		return composerRefusal(sres.Reason, sres.Message), nil
	}
	page, ok := sentEntry(meta, req.Text)
	if !ok {
		return composerRefusal(wire.ReasonSessionNotPlain, fmt.Sprintf("session %s is not a chat or a Design", req.SessionID)), nil
	}
	if len(req.Images) > wire.MaxPromptImages {
		return composerRefusal(wire.ReasonImageTooMany, fmt.Sprintf("at most %d images per prompt", wire.MaxPromptImages)), nil
	}
	dir, gate, err := p.imageSessionDir(req.SessionID)
	if err != nil {
		return ops.ComposerPickResult{}, err
	}
	if gate.Refused {
		return composerRefusal(gate.Reason, gate.Message), nil
	}
	picked, err := p.history.Load(drafthistory.Sent, req.EntryID)
	if errors.Is(err, os.ErrNotExist) {
		return composerRefusal(ops.ReasonHistoryEntryGone, "that entry is no longer in Sent messages"), nil
	}
	if err != nil {
		return ops.ComposerPickResult{}, err
	}
	picked.Paths = picked.Paths[:min(len(picked.Paths), wire.MaxPromptImages)]

	folder := filepath.Join(dir, imagesDir)
	p.imageMu.Lock()
	defer p.imageMu.Unlock()
	res, err := p.history.Restore(drafthistory.Sent, picked, page, folder, req.Images, func() error {
		_, err := p.history.Save(drafthistory.Sent, page, drafthistory.ImageFiles(folder, req.Images))
		return err
	})
	if err != nil {
		return ops.ComposerPickResult{}, err
	}
	out := ops.ComposerPickResult{Text: req.Text, Images: res.Images, Notices: []string{}}
	if out.Images == nil {
		out.Images = []string{}
	}
	if res.Changed {
		out.Text = picked.Entry.Text
		if res.Missing > 0 {
			out.Notices = append(out.Notices, drafthistory.MissingNotice(res.Missing))
		}
	}
	return out, nil
}
