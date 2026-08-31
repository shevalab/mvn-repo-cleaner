package repo

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// WriteList writes stale paths to a file, one absolute path per line.
func WriteList(path string, entries []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, e := range entries {
		fmt.Fprintln(w, e)
	}
	return w.Flush()
}

// ReadList reads a list file: one path per line, non-empty, trimmed.
func ReadList(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
