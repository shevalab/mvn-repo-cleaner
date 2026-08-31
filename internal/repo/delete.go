package repo

import (
	"os"
	"path/filepath"
	"strings"
)

// DeleteVersions removes the given version directories under the repo.
// Returns the list of successfully removed paths.
func DeleteVersions(paths []string) (removed []string, failed []string) {
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil {
			failed = append(failed, p)
			continue
		}
		removed = append(removed, p)
	}
	return removed, failed
}

// VersionDir returns a version directory path for a coordinate.
func VersionDir(repo string, groupID, artifactID, version string) string {
	group := strings.ReplaceAll(groupID, ".", string(filepath.Separator))
	return filepath.Join(repo, group, artifactID, version)
}
