package repo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shevalab/mvn-repo-cleaner/internal/model"
)

func mkVersion(t *testing.T, repo, group, artifact, version string) {
	t.Helper()
	dir := filepath.Join(repo, filepath.FromSlash(group), artifact, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, artifact+"-"+version+".jar"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWalk(t *testing.T) {
	repo := t.TempDir()
	mkVersion(t, repo, "com.example", "lib", "1.0")
	mkVersion(t, repo, "com.example", "lib", "2.0")
	mkVersion(t, repo, "org.other", "svc", "3.1")

	w, err := Walk(repo)
	if err != nil {
		t.Fatal(err)
	}
	lib := w[model.Artifact{GroupID: "com.example", ArtifactID: "lib"}]
	if len(lib) != 2 || lib[0] != "1.0" || lib[1] != "2.0" {
		t.Fatalf("lib versions = %v", lib)
	}
	svc := w[model.Artifact{GroupID: "org.other", ArtifactID: "svc"}]
	if len(svc) != 1 || svc[0] != "3.1" {
		t.Fatalf("svc versions = %v", svc)
	}
}

func TestExportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	list := filepath.Join(dir, "list.txt")
	entries := []string{"/a/b", "/c/d"}
	if err := WriteList(list, entries); err != nil {
		t.Fatal(err)
	}
	got, err := ReadList(list)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "/a/b" || got[1] != "/c/d" {
		t.Fatalf("got %v", got)
	}
}

func TestDeleteVersions(t *testing.T) {
	repo := t.TempDir()
	mkVersion(t, repo, "com.example", "lib", "1.0")
	dir := VersionDir(repo, "com.example", "lib", "1.0")
	removed, failed := DeleteVersions([]string{dir})
	if len(removed) != 1 || len(failed) != 0 {
		t.Fatalf("removed=%v failed=%v", removed, failed)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("dir should be gone, err=%v", err)
	}
}
