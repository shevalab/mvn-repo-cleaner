package model

import "strings"

// Artifact identifies a groupId:artifactId coordinate.
type Artifact struct {
	GroupID    string
	ArtifactID string
}

// KeepAllVersion is a sentinel version key used in in-use sets to mean "this
// artifact is in use but no concrete version could be resolved; keep every
// version on disk". It can never collide with a real Maven version.
const KeepAllVersion = "\x00*keep-all*"

func (a Artifact) String() string {
	return a.GroupID + ":" + a.ArtifactID
}

// Coordinate identifies a fully-resolved artifact version.
type Coordinate struct {
	Artifact
	Version string
}

func (c Coordinate) String() string {
	return c.GroupID + ":" + c.ArtifactID + ":" + c.Version
}

// DeclaredScopes are the scopes of a project's own declared dependencies that
// are relevant for retention. Provided and test dependencies are referenced in
// the build and must not be deleted, even though they are not on the runtime
// classpath.
var DeclaredScopes = map[string]bool{
	"compile":  true,
	"provided": true,
	"runtime":  true,
	"test":     true,
	"system":   true,
}

// PropagatedScopes are the scopes inherited when expanding a transitive
// dependency's own dependencies. Maven only propagates compile/runtime.
var PropagatedScopes = map[string]bool{
	"compile": true,
	"runtime": true,
}

// POM is the subset of a Maven POM needed for dependency resolution.
type POM struct {
	GroupID              string
	ArtifactID           string
	Version              string
	Packaging            string
	Parent               *Parent
	Properties           map[string]string
	Dependencies         []Dep
	DependencyManagement []ManagedDep
	Plugins              []Plugin
	PluginManagement     []Plugin
}

// Plugin is a build plugin coordinate declared in <build>.
type Plugin struct {
	GroupID    string
	ArtifactID string
	Version    string
}

// Parent references a parent POM coordinate.
type Parent struct {
	GroupID    string
	ArtifactID string
	Version    string
}

// Dep is a declared dependency.
type Dep struct {
	GroupID    string
	ArtifactID string
	Version    string
	Scope      string
	Exclusions []Exclusion
	Optional   bool
}

// Exclusion removes a transitive artifact.
type Exclusion struct {
	GroupID    string
	ArtifactID string
}

// ManagedDep is an entry in dependencyManagement.
type ManagedDep struct {
	GroupID    string
	ArtifactID string
	Version    string
	Scope      string
}

// NormalizeKey returns the repository path form for an artifact coordinate.
func (c Coordinate) RepoPath(repo string) string {
	group := strings.ReplaceAll(c.GroupID, ".", "/")
	return repo + "/" + group + "/" + c.ArtifactID + "/" + c.Version
}

// IsInUseScope reports whether a scope counts as a project's own declared
// dependency for retention.
func IsInUseScope(scope string) bool {
	return DeclaredScopes[scope]
}

// IsPropagatedScope reports whether a scope is inherited when expanding a
// transitive dependency's own dependencies.
func IsPropagatedScope(scope string) bool {
	return PropagatedScopes[scope]
}
