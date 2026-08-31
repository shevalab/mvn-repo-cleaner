package repo

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vadim/mvn-repo-cleaner/internal/model"
)

// Walk enumerates the artifacts and versions present under the repository root.
// Layout: <repo>/<group/path>/<artifact>/<version>. A version directory is
// identified by containing files of the form <artifact>-<version>.<ext>.
func Walk(repo string) (map[model.Artifact][]string, error) {
	result := map[model.Artifact][]string{}
	err := filepath.Walk(repo, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		// A version dir is at depth >= 3 and holds version files.
		if len(parts) < 3 {
			return nil
		}
		artifactID := parts[len(parts)-2]
		version := parts[len(parts)-1]
		if !looksLikeVersionDir(path, artifactID, version) {
			return nil
		}
		groupID := strings.Join(parts[:len(parts)-2], ".")
		c := model.Artifact{GroupID: groupID, ArtifactID: artifactID}
		result[c] = append(result[c], version)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for k := range result {
		sort.Strings(result[k])
	}
	return result, nil
}

// looksLikeVersionDir reports whether dir appears to be a Maven version
// directory by containing at least one file named <artifact>-<version>.<ext>.
func looksLikeVersionDir(dir, artifactID, version string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	prefix := artifactID + "-" + version + "."
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			return true
		}
	}
	return false
}
