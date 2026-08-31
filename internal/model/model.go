package model

import "strings"

// Artifact identifies a groupId:artifactId coordinate.
type Artifact struct {
	GroupID    string
	ArtifactID string
}

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

// Scopes is the set of dependency scopes that are relevant for a runtime
// build classpath. Others (test, provided, etc.) are not considered in-use
// resources that should be kept.
var Scopes = map[string]bool{
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

// IsInUseScope reports whether a scope counts as in-use for retention.
func IsInUseScope(scope string) bool {
	return Scopes[scope]
}
