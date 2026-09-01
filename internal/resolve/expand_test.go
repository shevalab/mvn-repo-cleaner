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

func TestResolveBuildPlugins(t *testing.T) {
	repo := t.TempDir()
	writePOM(t, repo, "org.plugin", "my-compiler-plugin", "3.0", `<project><groupId>org.plugin</groupId><artifactId>my-compiler-plugin</artifactId><version>3.0</version></project>`)
	root := filepath.Join(repo, "root", "pom.xml")
	os.MkdirAll(filepath.Dir(root), 0o755)
	os.WriteFile(root, []byte(`<project>
		<groupId>org.root</groupId><artifactId>root</artifactId><version>1.0</version>
		<build>
			<plugins>
				<plugin>
					<groupId>org.plugin</groupId><artifactId>my-compiler-plugin</artifactId><version>3.0</version>
				</plugin>
			</plugins>
			<pluginManagement>
				<plugins>
					<plugin>
						<groupId>org.plugin</groupId><artifactId>my-surefire-plugin</artifactId><version>2.4</version>
					</plugin>
				</plugins>
			</pluginManagement>
		</build>
	</project>`), 0o644)
	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if !has(inUse, "org.plugin", "my-compiler-plugin", "3.0") {
		t.Fatalf("expected build plugin in use, got %+v", inUse)
	}
	if !has(inUse, "org.plugin", "my-surefire-plugin", "2.4") {
		t.Fatalf("expected pluginManagement in use, got %+v", inUse)
	}
}

func TestResolveDefaultPluginGroup(t *testing.T) {
	repo := t.TempDir()
	writePOM(t, repo, defaultPluginGroup, "maven-compiler-plugin", "3.3", `<project><groupId>org.apache.maven.plugins</groupId><artifactId>maven-compiler-plugin</artifactId><version>3.3</version></project>`)
	root := filepath.Join(repo, "root", "pom.xml")
	os.MkdirAll(filepath.Dir(root), 0o755)
	os.WriteFile(root, []byte(`<project>
		<groupId>org.root</groupId><artifactId>root</artifactId><version>1.0</version>
		<build>
			<plugins>
				<plugin>
					<artifactId>maven-compiler-plugin</artifactId>
					<version>3.3</version>
				</plugin>
			</plugins>
		</build>
	</project>`), 0o644)
	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	// A plugin without <groupId> uses the org.apache.maven.plugins default, so
	// the on-disk coordinate must be matched.
	if !has(inUse, defaultPluginGroup, "maven-compiler-plugin", "3.3") {
		t.Fatalf("expected default plugin group in use, got %+v", inUse)
	}
}

func TestResolveBOMImport(t *testing.T) {
	repo := t.TempDir()
	// BOM manages a version; a project imports it and uses its managed dep.
	writePOM(t, repo, "org.bom", "bom", "1.0", `<project>
		<groupId>org.bom</groupId><artifactId>bom</artifactId><version>1.0</version><packaging>pom</packaging>
		<dependencyManagement>
			<dependencies>
				<dependency><groupId>org.x</groupId><artifactId>lib</artifactId><version>7.7</version></dependency>
			</dependencies>
		</dependencyManagement>
	</project>`)
	writePOM(t, repo, "org.x", "lib", "7.7", `<project><groupId>org.x</groupId><artifactId>lib</artifactId><version>7.7</version></project>`)
	root := filepath.Join(repo, "root", "pom.xml")
	os.MkdirAll(filepath.Dir(root), 0o755)
	os.WriteFile(root, []byte(`<project>
		<groupId>org.root</groupId><artifactId>root</artifactId><version>1.0</version>
		<dependencyManagement>
			<dependencies>
				<dependency><groupId>org.bom</groupId><artifactId>bom</artifactId><version>1.0</version><scope>import</scope></dependency>
			</dependencies>
		</dependencyManagement>
		<dependencies>
			<dependency><groupId>org.x</groupId><artifactId>lib</artifactId></dependency>
		</dependencies>
	</project>`), 0o644)
	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	// The BOM itself and the dependency it manages must both be in use.
	if !has(inUse, "org.bom", "bom", "1.0") {
		t.Fatalf("expected imported BOM in use, got %+v", inUse)
	}
	if !has(inUse, "org.x", "lib", "7.7") {
		t.Fatalf("expected BOM-managed dep in use, got %+v", inUse)
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

func TestResolveNonRuntimeScopes(t *testing.T) {
	repo := t.TempDir()
	writePOM(t, repo, "org.test", "junit", "4.13", `<project><groupId>org.test</groupId><artifactId>junit</artifactId><version>4.13</version></project>`)
	writePOM(t, repo, "org.prov", "lombok", "1.18", `<project><groupId>org.prov</groupId><artifactId>lombok</artifactId><version>1.18</version></project>`)
	root := filepath.Join(repo, "root", "pom.xml")
	os.MkdirAll(filepath.Dir(root), 0o755)
	os.WriteFile(root, []byte(`<project>
		<groupId>org.root</groupId><artifactId>root</artifactId><version>1.0</version>
		<dependencies>
			<dependency><groupId>org.test</groupId><artifactId>junit</artifactId><version>4.13</version><scope>test</scope></dependency>
			<dependency><groupId>org.prov</groupId><artifactId>lombok</artifactId><version>1.18</version><scope>provided</scope></dependency>
		</dependencies>
	</project>`), 0o644)
	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if !has(inUse, "org.test", "junit", "4.13") {
		t.Fatalf("test-scoped dep should be in-use, got %+v", inUse)
	}
	if !has(inUse, "org.prov", "lombok", "1.18") {
		t.Fatalf("provided-scoped dep should be in-use, got %+v", inUse)
	}
}

func TestResolveTransitiveScope(t *testing.T) {
	repo := t.TempDir()
	// a (compile) depends on b (test). Maven does not propagate test scope, so
	// b must NOT be treated as in-use when reached transitively.
	writePOM(t, repo, "org.a", "a", "1.0", `<project>
		<groupId>org.a</groupId><artifactId>a</artifactId><version>1.0</version>
		<dependencies>
			<dependency><groupId>org.b</groupId><artifactId>b</artifactId><version>1.0</version><scope>test</scope></dependency>
		</dependencies>
	</project>`)
	writePOM(t, repo, "org.b", "b", "1.0", `<project><groupId>org.b</groupId><artifactId>b</artifactId><version>1.0</version></project>`)
	rootPom := projectPOM(t, repo, "root", "1.0", `<dependency><groupId>org.a</groupId><artifactId>a</artifactId><version>1.0</version></dependency>`)
	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(rootPom)
	if err != nil {
		t.Fatal(err)
	}
	if !has(inUse, "org.a", "a", "1.0") {
		t.Fatalf("expected a in use, got %+v", inUse)
	}
	if has(inUse, "org.b", "b", "1.0") {
		t.Fatalf("test-scoped transitive dep should not propagate, got %+v", inUse)
	}
}

func TestResolveBOMImportPrecedence(t *testing.T) {
	repo := t.TempDir()
	// The imported BOM manages dep:1.0, but the project's own dependencyManagement
	// overrides it to dep:2.0. The project's own entry must win (Maven precedence).
	writePOM(t, repo, "org.bom", "bom", "1.0", `<project>
		<groupId>org.bom</groupId><artifactId>bom</artifactId><version>1.0</version><packaging>pom</packaging>
		<dependencyManagement>
			<dependencies>
				<dependency><groupId>org.z</groupId><artifactId>dep</artifactId><version>1.0</version></dependency>
			</dependencies>
		</dependencyManagement>
	</project>`)
	root := filepath.Join(repo, "root", "pom.xml")
	os.MkdirAll(filepath.Dir(root), 0o755)
	os.WriteFile(root, []byte(`<project>
		<groupId>org.root</groupId><artifactId>root</artifactId><version>1.0</version>
		<dependencyManagement>
			<dependencies>
				<dependency><groupId>org.bom</groupId><artifactId>bom</artifactId><version>1.0</version><scope>import</scope></dependency>
				<dependency><groupId>org.z</groupId><artifactId>dep</artifactId><version>2.0</version></dependency>
			</dependencies>
		</dependencyManagement>
		<dependencies>
			<dependency><groupId>org.z</groupId><artifactId>dep</artifactId></dependency>
		</dependencies>
	</project>`), 0o644)
	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if !has(inUse, "org.z", "dep", "2.0") {
		t.Fatalf("project's own dependencyManagement should override BOM, got %+v", inUse)
	}
	if has(inUse, "org.z", "dep", "1.0") {
		t.Fatalf("BOM-managed version should be overridden by project's own, got %+v", inUse)
	}
}

func has(set map[model.Artifact]map[string]bool, g, a, v string) bool {
	vers := set[model.Artifact{GroupID: g, ArtifactID: a}]
	return vers[v]
}

func TestResolveManagedOnlyKept(t *testing.T) {
	repo := t.TempDir()
	// A versionless-dep is NOT declared; only dependencyManagement references it.
	// The cleaner must still treat it as in-use because it is an explicit
	// declaration (matches the audit scripts' definition).
	root := filepath.Join(repo, "root", "pom.xml")
	os.MkdirAll(filepath.Dir(root), 0o755)
	os.WriteFile(root, []byte(`<project>
		<groupId>org.root</groupId><artifactId>root</artifactId><version>1.0</version>
		<dependencyManagement>
			<dependencies>
				<dependency><groupId>org.m</groupId><artifactId>managed-only</artifactId><version>5.0</version></dependency>
			</dependencies>
		</dependencyManagement>
	</project>`), 0o644)
	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if !has(inUse, "org.m", "managed-only", "5.0") {
		t.Fatalf("dependencyManagement-only entry should be in-use, got %+v", inUse)
	}
}

func TestResolveManagedUnresolvedVersionKeepAll(t *testing.T) {
	repo := t.TempDir()
	root := filepath.Join(repo, "root", "pom.xml")
	os.MkdirAll(filepath.Dir(root), 0o755)
	os.WriteFile(root, []byte(`<project>
		<groupId>org.root</groupId><artifactId>root</artifactId><version>1.0</version>
		<dependencyManagement>
			<dependencies>
				<dependency><groupId>org.m</groupId><artifactId>unresolved</artifactId><version>${missing.prop}</version></dependency>
			</dependencies>
		</dependencyManagement>
	</project>`), 0o644)
	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	vers := inUse[model.Artifact{GroupID: "org.m", ArtifactID: "unresolved"}]
	if !vers[model.KeepAllVersion] {
		t.Fatalf("unresolved managed version should keep all versions, got %+v", vers)
	}
}

func TestResolvePluginArtifactItems(t *testing.T) {
	repo := t.TempDir()
	root := filepath.Join(repo, "root", "pom.xml")
	os.MkdirAll(filepath.Dir(root), 0o755)
	os.WriteFile(root, []byte(`<project>
		<groupId>org.root</groupId><artifactId>root</artifactId><version>1.0</version>
		<properties>
			<webclient.version>0.0.216</webclient.version>
		</properties>
		<build>
			<plugins>
				<plugin>
					<groupId>org.apache.maven.plugins</groupId>
					<artifactId>maven-dependency-plugin</artifactId>
					<version>3.6.0</version>
					<executions><execution><goals><goal>unpack</goal></goals></execution></executions>
					<configuration>
						<artifactItems>
							<artifactItem>
								<groupId>com.acme</groupId>
								<artifactId>web-client</artifactId>
								<version>${webclient.version}</version>
								<type>zip</type>
							</artifactItem>
						</artifactItems>
					</configuration>
				</plugin>
			</plugins>
		</build>
	</project>`), 0o644)
	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if !has(inUse, "com.acme", "web-client", "0.0.216") {
		t.Fatalf("artifactItem reference should be in-use, got %+v", inUse)
	}
}

func TestResolveProfiles(t *testing.T) {
	repo := t.TempDir()
	root := filepath.Join(repo, "root", "pom.xml")
	os.MkdirAll(filepath.Dir(root), 0o755)
	os.WriteFile(root, []byte(`<project>
		<groupId>org.root</groupId><artifactId>root</artifactId><version>1.0</version>
		<profiles>
			<profile>
				<id>prod</id>
				<dependencies>
					<dependency><groupId>org.p</groupId><artifactId>prod-dep</artifactId><version>2.0</version></dependency>
				</dependencies>
				<dependencyManagement>
					<dependencies>
						<dependency><groupId>org.p</groupId><artifactId>managed-dep</artifactId><version>3.0</version></dependency>
					</dependencies>
				</dependencyManagement>
				<build><plugins>
					<plugin><groupId>org.p</groupId><artifactId>prof-plugin</artifactId><version>4.0</version></plugin>
				</plugins></build>
			</profile>
		</profiles>
	</project>`), 0o644)
	l := &Loader{Repo: repo}
	inUse, err := l.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][3]string{
		{"org.p", "prod-dep", "2.0"},
		{"org.p", "managed-dep", "3.0"},
		{"org.p", "prof-plugin", "4.0"},
	} {
		if !has(inUse, want[0], want[1], want[2]) {
			t.Fatalf("profile reference %s:%s:%s should be in-use, got %+v", want[0], want[1], want[2], inUse)
		}
	}
}
