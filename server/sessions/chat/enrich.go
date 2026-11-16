package chat

import "github.com/elentok/gx/transcript"

// taskReads replaces the per-file readers of the task enrichers. Tests count
// reads with them; nil means the real reader.
type taskReads struct {
	shellOutput func(path string) (ShellOutput, error)
	subagent    func(path string) (transcript.SubagentDetails, error)
	codexChild  func(path string) (codexChild, error)
}

// enrichSpec says how one kind of record picks its file and takes the details.
type enrichSpec[T comparable] struct {
	cache   *FileCache[T]
	compute func(path string) (T, error)
	// path is the file of r, or false when r takes no details.
	path func(r transcript.TaskRecord) (string, bool)
	// usable reports whether the read details carry anything.
	usable func(T) bool
	merge  func(r *transcript.TaskRecord, d T)
}

// enrichTasks returns records with the details of each record's file merged in.
// It copies the slice on the first change: the records belong to the cached
// snapshot.
func enrichTasks[T comparable](records []transcript.TaskRecord, s enrichSpec[T]) []transcript.TaskRecord {
	var out []transcript.TaskRecord
	for i, r := range records {
		path, ok := s.path(r)
		if !ok {
			continue
		}
		d, err := s.cache.Read(path, s.compute)
		if err != nil || !s.usable(d) {
			continue
		}
		if out == nil {
			out = make([]transcript.TaskRecord, len(records))
			copy(out, records)
		}
		s.merge(&r, d)
		out[i] = r
	}
	if out == nil {
		return records
	}
	return out
}
