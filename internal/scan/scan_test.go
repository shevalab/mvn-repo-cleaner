package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vadim/mvn-repo-cleaner/internal/model"
)

func TestFindPomsRecursive(t *testing.T) {
	root := t.TempDir()
	poms := []string{
		"a/pom.xml",
		"a/mod/pom.xml",
		"b/pom.xml",
		"target/build/pom.xml", // skipped (inside target)
		".git/x/pom.xml",       // skipped (git)
	}
	for _, p := range poms {
		dir := filepath.Join(root, filepath.Dir(p))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte("<project/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := FindPoms(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 poms, got %d: %v", len(got), got)
	}
}

func TestInUseSetMerge(t *testing.T) {
	s := NewInUseSet()
	a := model.Artifact{GroupID: "g", ArtifactID: "a"}
	s.Add(map[model.Artifact]map[string]bool{a: {"1.0": true, "2.0": true}})
	s.Add(map[model.Artifact]map[string]bool{a: {"2.0": true, "3.0": true}})
	if !s[a]["1.0"] || !s[a]["2.0"] || !s[a]["3.0"] {
		t.Fatalf("union failed: %+v", s[a])
	}
}
