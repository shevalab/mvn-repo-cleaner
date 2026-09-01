package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shevalab/mvn-repo-cleaner/internal/cli"
	"github.com/shevalab/mvn-repo-cleaner/internal/repo"
)

func writePom(t *testing.T, repo, group, artifact, version string) {
	t.Helper()
	groupPath := strings.ReplaceAll(group, ".", "/")
	dir := filepath.Join(repo, filepath.FromSlash(groupPath), artifact, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `<project>
		<groupId>` + group + `</groupId><artifactId>` + artifact + `</artifactId><version>` + version + `</version>
	</project>`
	if err := os.WriteFile(filepath.Join(dir, artifact+"-"+version+".pom"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, artifact+"-"+version+".jar"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeProject(t *testing.T, projectDir, name string, deps string) {
	t.Helper()
	dir := filepath.Join(projectDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `<project>
		<groupId>org.app</groupId><artifactId>` + name + `</artifactId><version>1.0</version>
		<dependencies>` + deps + `</dependencies>
	</project>`
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestEndToEndStale builds a repo with used + stale artifacts, a project that
// uses some, and verifies computeStale reports exactly the stale ones.
func TestEndToEndStale(t *testing.T) {
	repodir := t.TempDir()
	projectRoot := t.TempDir()

	// Used: com.used:api:2.0
	writePom(t, repodir, "com.used", "api", "2.0")
	// Stale version: com.used:api:1.0 (not referenced)
	writePom(t, repodir, "com.used", "api", "1.0")
	// Fully stale artifact: com.gone:lib:3.0
	writePom(t, repodir, "com.gone", "lib", "3.0")
	// Transitive: com.trans:dep:1.5, used by com.used:api's pom
	writePom(t, repodir, "com.trans", "dep", "1.5")

	// Point api:2.0's pom at the transitive dep so resolution follows it.
	apiPom := filepath.Join(repodir, "com", "used", "api", "2.0", "api-2.0.pom")
	os.WriteFile(apiPom, []byte(`<project>
		<groupId>com.used</groupId><artifactId>api</artifactId><version>2.0</version>
		<dependencies>
			<dependency><groupId>com.trans</groupId><artifactId>dep</artifactId><version>1.5</version></dependency>
		</dependencies>
	</project>`), 0o644)

	// Project uses com.used:api:2.0 only.
	writeProject(t, projectRoot, "proj", `<dependency><groupId>com.used</groupId><artifactId>api</artifactId><version>2.0</version></dependency>`)

	cfg := &cli.Config{Paths: []string{projectRoot}, Repo: repodir, Mode: "scan"}
	stale := computeStale(cfg)

	got := map[string]bool{}
	for _, p := range stale {
		got[filepath.Base(filepath.Dir(p))+"@"+filepath.Base(p)] = true
	}
	want := map[string]bool{
		"api@1.0": true, // stale version of used artifact
		"lib@3.0": true, // unused artifact
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 stale, got %v", got)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing stale %s", k)
		}
	}
	if got["api@2.0"] {
		t.Error("api:2.0 is in use and should not be stale")
	}
	if got["dep@1.5"] {
		t.Error("transitive dep 1.5 is in use and should not be stale")
	}

	// Test actual deletion.
	removed, failed := repo.DeleteVersions(stale)
	if len(failed) != 0 {
		t.Fatalf("delete failed: %v", failed)
	}
	if len(removed) != 2 {
		t.Fatalf("expected 2 removed, got %d", len(removed))
	}
	if _, err := os.Stat(filepath.Join(repodir, "com", "used", "api", "1.0")); !os.IsNotExist(err) {
		t.Error("api:1.0 should be deleted")
	}
	if _, err := os.Stat(filepath.Join(repodir, "com", "used", "api", "2.0")); err != nil {
		t.Error("api:2.0 should remain")
	}
}

// TestEndToEndStaleUnknownVersion covers the conservative path: a dependency
// whose version cannot be concretely resolved (missing parent/management) is
// still treated as in use, so ALL of its versions are kept.
func TestEndToEndStaleUnknownVersion(t *testing.T) {
	repodir := t.TempDir()
	projectRoot := t.TempDir()

	// org.loose:lib exists in two versions on disk; a project declares
	// org.loose:lib with no version and no resolvable management, so the
	// resolver cannot pin a concrete version.
	writePom(t, repodir, "org.loose", "lib", "1.0")
	writePom(t, repodir, "org.loose", "lib", "2.0")

	// Project declares the versionless dependency but its parent/management
	// are absent from the repo, so resolution marks lib in-use w/o a version.
	dir := filepath.Join(projectRoot, "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(`<project>
		<groupId>org.app</groupId><artifactId>proj</artifactId><version>1.0</version>
		<dependencies>
			<dependency><groupId>org.loose</groupId><artifactId>lib</artifactId></dependency>
		</dependencies>
	</project>`), 0o644)

	cfg := &cli.Config{Paths: []string{projectRoot}, Repo: repodir, Mode: "scan"}
	stale := computeStale(cfg)

	for _, p := range stale {
		if strings.Contains(p, "/org/loose/lib/") {
			t.Fatalf("org.loose:lib is conservative-in-use and must not be stale, got %s", p)
		}
	}
}
