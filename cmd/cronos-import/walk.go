package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func collect(paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			out = append(out, p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".jrxml") {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	// Sorted so two runs over one estate produce the same names for the same
	// files — the suffix a collision gets depends on which was seen first.
	sort.Strings(out)
	return out, nil
}

// taken hands out names that are unique per kind.
type taken struct{ seen map[string]bool }

func (t *taken) pick(kind, want string) (string, bool) {
	if t.seen == nil {
		t.seen = map[string]bool{}
	}
	key := kind + "/" + want
	if !t.seen[key] {
		t.seen[key] = true
		return want, false
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", want, n)
		if k := kind + "/" + candidate; !t.seen[k] {
			t.seen[k] = true
			return candidate, true
		}
	}
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// was is the -s a verb takes for one subject: "1 file needs", "2 files need".
func was(n int) string {
	if n == 1 {
		return "s"
	}
	return ""
}

func has(n int) string {
	if n == 1 {
		return "has"
	}
	return "have"
}

func they(n int) string {
	if n == 1 {
		return "it is"
	}
	return "they are"
}
