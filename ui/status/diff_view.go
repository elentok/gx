package status

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui/status/diffarea"
)

func (m *Model) diffPaneInput() diffarea.PaneInput {
	in := diffarea.PaneInput{
		DiffFocused: m.focus == focusDiff,
		NerdFont:    m.settings.UseNerdFontIcons,
		BinaryLines: m.appendBinaryDiffLines,
	}
	if file, ok := m.selectedStatusFile(); ok {
		in.File = diffarea.DisplayedPath(file.stageFile)
	}
	return in
}

func (m *Model) renderDiffPane(width, height int) string {
	return m.diffarea.Render(width, height, m.diffPaneInput())
}

func (m *Model) syncDiffViewports() {
	_, diffW := m.splitWidth()
	filetreeH, diffH := splitHeight(m.width, mainContentHeight(m.height))
	if m.diffarea.Fullscreen && m.focus == focusDiff {
		diffW = m.width
	}
	m.diffarea.SyncForSize(diffW, diffH)
	m.filetree.model.SetVisibleHeight(m.filetree.innerHeight(filetreeH, filetreeViewOf(*m)))
}

func (m Model) binarySummaryLine() string {
	file, ok := m.selectedStatusFile()
	if !ok {
		return "binary file"
	}
	prevSize, newSize, prevOK, newOK := stage.BinaryFileSizes(m.worktreeRoot, file.stageFile)
	if !prevOK && !newOK {
		return "binary file"
	}
	return fmt.Sprintf("binary file (prev size: %s, new size: %s)", formatSize(prevSize, prevOK), formatSize(newSize, newOK))
}

// isWorktreeSymlink reports whether the file at relPath inside worktreeRoot is a
// symlink in the working tree. Returns false if the file does not exist or any
// error occurs (e.g. deleted files).
func isWorktreeSymlink(worktreeRoot, relPath string) bool {
	info, err := os.Lstat(filepath.Join(worktreeRoot, filepath.FromSlash(relPath)))
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

func formatSize(size int64, ok bool) string {
	if !ok {
		return "n/a"
	}
	if size < 0 {
		size = 0
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v := float64(size)
	idx := 0
	for v >= 1024 && idx < len(units)-1 {
		v /= 1024
		idx++
	}
	if idx == 0 {
		return fmt.Sprintf("%d %s", size, units[idx])
	}
	if v >= 10 {
		return fmt.Sprintf("%.0f %s", v, units[idx])
	}
	return fmt.Sprintf("%.1f %s", v, units[idx])
}
