package bindings

import (
	"fmt"
	"os"
)

// PathHealth is a coarse check of a bound path (no watching).
type PathHealth string

const (
	PathOK      PathHealth = "ok"
	PathMissing PathHealth = "missing"
	PathNotDir  PathHealth = "not_a_directory"
	PathError   PathHealth = "error"
)

// CheckPath reports whether localPath is currently a usable directory.
func CheckPath(localPath string) (PathHealth, string) {
	if localPath == "" {
		return PathError, "empty path"
	}
	info, err := os.Stat(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return PathMissing, "does not exist"
		}
		return PathError, err.Error()
	}
	if !info.IsDir() {
		return PathNotDir, "not a directory"
	}
	return PathOK, "directory present"
}

// StatusRow is one binding plus live path health for display.
type StatusRow struct {
	Binding Binding
	Health  PathHealth
	Detail  string
}

func (r StatusRow) String() string {
	sub := "no"
	if r.Binding.Subscribed {
		sub = "yes"
	}
	name := r.Binding.Name
	if name == "" {
		name = "-"
	}
	return fmt.Sprintf("%s  %-16s  subscribed=%-3s  health=%-16s  %s  (%s)",
		r.Binding.FolderID, name, sub, r.Health, r.Binding.LocalPath, r.Detail)
}

// Status returns health for every stored binding. Does not use fsnotify.
func (s *Store) Status() ([]StatusRow, error) {
	list, err := s.List()
	if err != nil {
		return nil, err
	}
	out := make([]StatusRow, 0, len(list))
	for _, b := range list {
		h, detail := CheckPath(b.LocalPath)
		out = append(out, StatusRow{Binding: b, Health: h, Detail: detail})
	}
	return out, nil
}
