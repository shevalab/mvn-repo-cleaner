package resolve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shevalab/mvn-repo-cleaner/internal/model"
)

// writePOM writes a pom.xml under repoDir/<group path>/<artifact>/<version>/
// following the real Maven layout (group dots split into path segments).
func writePOM(t *testing.T, repo string, group, artifact, version, content string) {
	t.Helper()
	groupPath := strings.ReplaceAll(group, ".", "/")
	dir := filepath.Join(repo, filepath.FromSlash(groupPath), artifact, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, artifact+"-"+version+".pom"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParsePOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	content := `<project>
	<groupId>com.example</groupId>
	<artifactId>root</artifactId>
	<version>1.0</version>
	<properties><maven.compiler>11</maven.compiler></properties>
	<dependencies>
		<dependency>
			<groupId>org.foo</groupId>
			<artifactId>bar</artifactId>
			<version>2.0</version>
		</dependency>
	</dependencies>
</project>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := ParsePOM(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.GroupID != "com.example" || p.ArtifactID != "root" || p.Version != "1.0" {
		t.Fatalf("bad coords: %+v", p)
	}
	if len(p.Dependencies) != 1 || p.Dependencies[0].GroupID != "org.foo" {
		t.Fatalf("bad deps: %+v", p.Dependencies)
	}
}

func TestResolveTransitiveAndCycle(t *testing.T) {
	repo := t.TempDir()
	// a depends on b; b depends on a (cycle). Resolver must terminate.
	writePOM(t, repo, "org.a", "a", "1.0", `<project>
		<groupId>org.a</groupId><artifactId>a</artifactId><version>1.0</version>
		<dependencies>
			<dependency><groupId>org.b</groupId><artifactId>b</artifactId><version>1.0</version></dependency>
		</dependencies>
	</project>`)
	writePOM(t, repo, "org.b", "b", "1.0", `<project>
		<groupId>org.b</groupId><artifactId>b</artifactId><version>1.0</version>
		<dependencies>
			<dependency><groupId>org.a</groupId><artifactId>a</artifactId><version>1.0</version></dependency>
		</dependencies>
	</project>`)

	rootPom := projectPOM(t, repo, "root", "1.0", `<dependency><groupId>org.a</groupId><artifactId>a</artifactId><version>1.0</version></dependency>`)

	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(rootPom)
	if err != nil {
		t.Fatal(err)
	}
	if !has(inUse, "org.a", "a", "1.0") || !has(inUse, "org.b", "b", "1.0") {
		t.Fatalf("expected a and b in use, got %+v", inUse)
	}
}

func TestResolveExclusion(t *testing.T) {
	repo := t.TempDir()
	// a depends on b and c; a declares b excluded from c.
	// c depends on b. Exclusion on c->b means b not pulled via c, but
	// b is still pulled directly from a.
	writePOM(t, repo, "org.a", "a", "1.0", `<project>
		<groupId>org.a</groupId><artifactId>a</artifactId><version>1.0</version>
		<dependencies>
			<dependency><groupId>org.b</groupId><artifactId>b</artifactId><version>1.0</version></dependency>
			<dependency><groupId>org.c</groupId><artifactId>c</artifactId><version>1.0</version>
				<exclusions><exclusion><groupId>org.b</groupId><artifactId>b</artifactId></exclusion></exclusions>
			</dependency>
		</dependencies>
	</project>`)
	writePOM(t, repo, "org.c", "c", "1.0", `<project>
		<groupId>org.c</groupId><artifactId>c</artifactId><version>1.0</version>
		<dependencies>
			<dependency><groupId>org.b</groupId><artifactId>b</artifactId><version>1.0</version></dependency>
		</dependencies>
	</project>`)
	writePOM(t, repo, "org.b", "b", "1.0", `<project>
		<groupId>org.b</groupId><artifactId>b</artifactId><version>1.0</version></project>`)

	rootPom := projectPOM(t, repo, "root", "1.0", `<dependency><groupId>org.a</groupId><artifactId>a</artifactId><version>1.0</version></dependency>`)

	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(rootPom)
	if err != nil {
		t.Fatal(err)
	}
	if !has(inUse, "org.a", "a", "1.0") || !has(inUse, "org.c", "c", "1.0") || !has(inUse, "org.b", "b", "1.0") {
		t.Fatalf("b excluded from c but still direct: got %+v", inUse)
	}
}

func TestResolveMissingPOMConservative(t *testing.T) {
	repo := t.TempDir()
	// root depends on a:1.0 which is NOT in the repo (pom absent).
	rootPom := projectPOM(t, repo, "root", "1.0", `<dependency><groupId>org.missing</groupId><artifactId>a</artifactId><version>1.0</version></dependency>`)
	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(rootPom)
	if err != nil {
		t.Fatal(err)
	}
	// a:1.0 is declared -> in use, even though pom absent.
	if !has(inUse, "org.missing", "a", "1.0") {
		t.Fatalf("declared missing pom should be in-use, got %+v", inUse)
	}
}

func TestResolveRange(t *testing.T) {
	repo := t.TempDir()
	writePOM(t, repo, "org.r", "r", "1.0", `<project><groupId>org.r</groupId><artifactId>r</artifactId><version>1.0</version></project>`)
	writePOM(t, repo, "org.r", "r", "2.5", `<project><groupId>org.r</groupId><artifactId>r</artifactId><version>2.5</version></project>`)
	writePOM(t, repo, "org.r", "r", "3.0", `<project><groupId>org.r</groupId><artifactId>r</artifactId><version>3.0</version></project>`)

	rootPom := projectPOM(t, repo, "root", "1.0", `<dependency><groupId>org.r</groupId><artifactId>r</artifactId><version>[2.0,3.0)</version></dependency>`)
	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(rootPom)
	if err != nil {
		t.Fatal(err)
	}
	if !has(inUse, "org.r", "r", "2.5") {
		t.Fatalf("expected r:2.5 resolved, got %+v", inUse)
	}
	if has(inUse, "org.r", "r", "3.0") {
		t.Fatalf("3.0 outside range should not be in use")
	}
}

func TestResolveDependencyManagement(t *testing.T) {
	repo := t.TempDir()
	writePOM(t, repo, "org.dm", "dep", "9.9", `<project><groupId>org.dm</groupId><artifactId>dep</artifactId><version>9.9</version></project>`)
	// root uses dep with no version, managed by dependencyManagement.
	root := filepath.Join(repo, "root", "pom.xml")
	os.MkdirAll(filepath.Dir(root), 0o755)
	os.WriteFile(root, []byte(`<project>
		<groupId>org.root</groupId><artifactId>root</artifactId><version>1.0</version>
		<dependencyManagement>
			<dependencies>
				<dependency><groupId>org.dm</groupId><artifactId>dep</artifactId><version>9.9</version></dependency>
			</dependencies>
		</dependencyManagement>
		<dependencies>
			<dependency><groupId>org.dm</groupId><artifactId>dep</artifactId></dependency>
		</dependencies>
	</project>`), 0o644)
	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if !has(inUse, "org.dm", "dep", "9.9") {
		t.Fatalf("expected managed version 9.9, got %+v", inUse)
	}
}

// projectPOM writes a root-style project pom with the given dependency block.
func projectPOM(t *testing.T, repo, name, version, deps string) string {
	t.Helper()
	dir := filepath.Join(repo, "projects", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pom.xml")
	content := `<project>
		<groupId>org.root</groupId><artifactId>` + name + `</artifactId><version>` + version + `</version>
		<dependencies>` + deps + `</dependencies>
	</project>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func has(set map[model.Artifact]map[string]bool, g, a, v string) bool {
	vers := set[model.Artifact{GroupID: g, ArtifactID: a}]
	return vers[v]
}
