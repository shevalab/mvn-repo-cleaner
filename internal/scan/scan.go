package scan

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/vadim/mvn-repo-cleaner/internal/model"
	"github.com/vadim/mvn-repo-cleaner/internal/resolve"
)

// FindPoms walks root recursively and returns all pom.xml file paths.
func FindPoms(root string) ([]string, error) {
	var poms []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			// Skip hidden dirs and VCS dirs.
			switch d.Name() {
			case ".git", ".svn", ".hg", "target", ".idea", ".gradle", ".m2":
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == "pom.xml" {
			poms = append(poms, path)
		}
		return nil
	})
	return poms, err
}

// InUseSet accumulates per-artifact versions across many resolved projects.
type InUseSet map[model.Artifact]map[string]bool

// NewInUseSet returns an empty set.
func NewInUseSet() InUseSet {
	return InUseSet{}
}

// Add merges a resolved project's in-use map into the set.
func (s InUseSet) Add(project map[model.Artifact]map[string]bool) {
	for a, versions := range project {
		if _, ok := s[a]; !ok {
			s[a] = map[string]bool{}
		}
		for v := range versions {
			s[a][v] = true
		}
	}
}

// ScanProject resolves a single project's root pom into in-use versions.
func ScanProject(loader *resolve.Loader, pomPath string) (map[model.Artifact]map[string]bool, error) {
	return loader.Resolve(pomPath)
}

// FileExists reports whether a path exists.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
