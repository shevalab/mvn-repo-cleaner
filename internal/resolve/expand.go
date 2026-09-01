package resolve

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/shevalab/mvn-repo-cleaner/internal/model"
)

// Loader loads POMs from a local Maven repository.
type Loader struct {
	Repo string
}

var errNoPOM = &noPOMError{}

type noPOMError struct{}

func (e *noPOMError) Error() string { return "incomplete coordinate: no pom" }

// Resolve computes the full transitive in-use set (artifact -> versions) for a
// scanned root pom file, including parent POMs which are used for inheritance.
func (l *Loader) Resolve(rootPom string) (map[model.Artifact]map[string]bool, error) {
    inUse := map[model.Artifact]map[string]bool{}
    root, err := ParsePOM(rootPom)
    if err != nil {
        return nil, err
    }
    // Add any parent hierarchy as in-use artifacts.
    if err := l.addParentChain(root, inUse); err != nil {
        // continue even if parent chain can't be fully resolved
    }
    eff, err := l.effective(root, map[string]string{})
    if err != nil {
        // Even if effective fails, keep whatever coordinate we can.
        if eff == nil {
            return inUse, nil
        }
    }
    if eff.Coord.GroupID != "" && eff.Coord.Version != "" {
        addVersion(inUse, eff.Coord)
    }
    // Record build plugins, pluginManagement, and imported BOMs as in-use so the
    // cleaner does not delete the build toolchain coordinates.
    for _, pl := range eff.PluginMgr {
        addVersion(inUse, model.Coordinate{Artifact: model.Artifact{GroupID: pl.GroupID, ArtifactID: pl.ArtifactID}, Version: pl.Version})
    }
    for _, pl := range eff.Plugins {
        addVersion(inUse, model.Coordinate{Artifact: model.Artifact{GroupID: pl.GroupID, ArtifactID: pl.ArtifactID}, Version: pl.Version})
    }
    for _, b := range eff.BOMs {
        addVersion(inUse, b)
    }
    visited := map[string]bool{}
    l.expand(eff, nil, visited, inUse, true)
    return inUse, nil
}

// addParentChain walks the parent chain of a POM and records each parent artifact/version as in‑use.
func (l *Loader) addParentChain(p *model.POM, inUse map[model.Artifact]map[string]bool) error {
    // Resolve properties for interpolation of the parent fields.
    props := map[string]string{}
    if p.Properties != nil {
        for k, v := range p.Properties {
            props[k] = v
        }
    }
    for p.Parent != nil {
        pg := interp(p.Parent.GroupID, props)
        pa := interp(p.Parent.ArtifactID, props)
        pv := interp(p.Parent.Version, props)
        if pg == "" || pa == "" || pv == "" {
            break
        }
        addVersion(inUse, model.Coordinate{Artifact: model.Artifact{GroupID: pg, ArtifactID: pa}, Version: pv})
        // Load the parent POM to continue up the chain.
        parentPOM, err := l.parseFromRepo(pg, pa, pv)
        if err != nil {
            return err
        }
        // Merge parent properties for next iteration.
        for k, v := range parentPOM.Properties {
            if _, exists := props[k]; !exists {
                props[k] = v
            }
        }
        p = parentPOM
    }
    return nil
}

// effectivePOM is a parent-resolved POM with merged properties and management.
type effectivePOM struct {
	Coord      model.Coordinate
	Properties map[string]string
	Depts      []model.Dep
	Managed    map[model.Artifact]model.ManagedDep
	Plugins    []model.Plugin
	PluginMgr  map[model.Artifact]model.Plugin
	BOMs       []model.Coordinate
}

// effective resolves a POM: walks the parent chain to inherit coordinates,
// properties, and dependencyManagement.
func (l *Loader) effective(p *model.POM, inherited map[string]string) (*effectivePOM, error) {
	props := map[string]string{}
	for k, v := range inherited {
		props[k] = v
	}
	for k, v := range p.Properties {
		props[k] = v
	}

	group, artifact, version := p.GroupID, p.ArtifactID, p.Version
	managed := map[model.Artifact]model.ManagedDep{}
	pluginMgr := map[model.Artifact]model.Plugin{}
	var inheritedPlugins []model.Plugin
	var inheritedBOMs []model.Coordinate

	// Resolve the parent chain first.
	if p.Parent != nil {
		pg := interp(p.Parent.GroupID, props)
		pa := interp(p.Parent.ArtifactID, props)
		pv := interp(p.Parent.Version, props)
		if parentPOM, err := l.parseFromRepo(pg, pa, pv); err == nil {
			parentEff, err := l.effective(parentPOM, props)
			if err == nil {
				// Inherit properties (child overrides parent).
				for k, v := range parentEff.Properties {
					if _, exists := props[k]; !exists {
						props[k] = v
					}
				}
				for a, m := range parentEff.Managed {
					managed[a] = m
				}
				for a, m := range parentEff.PluginMgr {
					pluginMgr[a] = m
				}
				// Inherit parent build plugins and imported BOMs.
				inheritedPlugins = append(inheritedPlugins, parentEff.Plugins...)
				inheritedBOMs = append(inheritedBOMs, parentEff.BOMs...)
				if group == "" {
					group = parentEff.Coord.GroupID
				}
				if version == "" {
					version = parentEff.Coord.Version
				}
			}
		}
	}

	// Re-interpret coordinates fully with merged props.
	props["project.groupId"] = group
	props["project.artifactId"] = artifact
	props["project.version"] = version
	if version != "" {
		props["pom.version"] = version
	}
	if group != "" {
		group = interp(group, props)
	}
	version = interp(version, props)

	eff := &effectivePOM{
		Coord: model.Coordinate{
			Artifact: model.Artifact{GroupID: group, ArtifactID: artifact},
			Version:  version,
		},
		Properties: props,
		Managed:    managed,
		PluginMgr:  pluginMgr,
	}

	eff.Depts = p.Dependencies
	// Apply this POM's own dependencyManagement (overrides inherited).
	for _, m := range p.DependencyManagement {
		mm := model.ManagedDep{
			GroupID:    interp(m.GroupID, props),
			ArtifactID: interp(m.ArtifactID, props),
			Version:    interp(m.Version, props),
			Scope:      m.Scope,
		}
		managed[model.Artifact{GroupID: mm.GroupID, ArtifactID: mm.ArtifactID}] = mm
	}
	// Import-scope BOMs: merge their dependencyManagement and keep them in use.
	eff.BOMs = append(eff.BOMs, inheritedBOMs...)
	for _, m := range p.DependencyManagement {
		if m.Scope != "import" {
			continue
		}
		mg := interp(m.GroupID, props)
		ma := interp(m.ArtifactID, props)
		mv := interp(m.Version, props)
		if mg == "" || ma == "" || mv == "" {
			continue
		}
		bomCoord := model.Coordinate{Artifact: model.Artifact{GroupID: mg, ArtifactID: ma}, Version: mv}
		eff.BOMs = append(eff.BOMs, bomCoord)
		l.applyBOM(bomCoord, props, managed, &eff.BOMs)
	}
	// Apply this POM's own pluginManagement (overrides inherited).
	for _, pl := range p.PluginManagement {
		pp := model.Plugin{
			GroupID:    interp(pl.GroupID, props),
			ArtifactID: interp(pl.ArtifactID, props),
			Version:    interp(pl.Version, props),
		}
		pluginMgr[model.Artifact{GroupID: pp.GroupID, ArtifactID: pp.ArtifactID}] = pp
	}
	// This POM's own build plugins (inherited first so build order matches).
	eff.Plugins = append(eff.Plugins, inheritedPlugins...)
	for _, pl := range p.Plugins {
		eff.Plugins = append(eff.Plugins, model.Plugin{
			GroupID:    interp(pl.GroupID, props),
			ArtifactID: interp(pl.ArtifactID, props),
			Version:    interp(pl.Version, props),
		})
	}
	return eff, nil
}

func (l *Loader) parseFromRepo(group, artifact, version string) (*model.POM, error) {
	if group == "" || artifact == "" || version == "" {
		return nil, errNoPOM
	}
	path := filepath.Join(l.RepoDir(model.Coordinate{Artifact: model.Artifact{GroupID: group, ArtifactID: artifact}, Version: version}), artifact+"-"+version+".pom")
	return ParsePOM(path)
}

// applyBOM loads an import-scoped BOM, merges its dependencyManagement into the
// managed map, and recurses into any nested BOMs it imports.
func (l *Loader) applyBOM(bom model.Coordinate, props map[string]string, managed map[model.Artifact]model.ManagedDep, boms *[]model.Coordinate) {
	pom, err := l.parseFromRepo(bom.GroupID, bom.ArtifactID, bom.Version)
	if err != nil {
		return
	}
	for _, m := range pom.DependencyManagement {
		mg := interp(m.GroupID, props)
		ma := interp(m.ArtifactID, props)
		mv := interp(m.Version, props)
		mm := model.ManagedDep{GroupID: mg, ArtifactID: ma, Version: mv, Scope: m.Scope}
		managed[model.Artifact{GroupID: mg, ArtifactID: ma}] = mm
		if m.Scope == "import" && mg != "" && ma != "" && mv != "" {
			nested := model.Coordinate{Artifact: model.Artifact{GroupID: mg, ArtifactID: ma}, Version: mv}
			*boms = append(*boms, nested)
			l.applyBOM(nested, props, managed, boms)
		}
	}
}

// RepoDir returns the version directory path for a coordinate. Maven stores
// the group with each dot split into a path segment (e.g. com.example -> com/example).
func (l *Loader) RepoDir(c model.Coordinate) string {
	return repoDir(l.Repo, c.GroupID, c.ArtifactID, c.Version)
}

func repoDir(repo, group, artifact, version string) string {
	groupPath := strings.ReplaceAll(group, ".", "/")
	return filepath.Join(repo, filepath.FromSlash(groupPath), artifact, version)
}

// expand walks the dependency tree, populating inUse. For the root project all
// declared scopes count; for transitive dependencies only propagated (compile/
// runtime) scopes do.
func (l *Loader) expand(eff *effectivePOM, excludes map[model.Artifact]bool, visited map[string]bool, inUse map[model.Artifact]map[string]bool, root bool) {
	for _, d := range eff.Depts {
		if root && !model.IsInUseScope(d.Scope) {
			continue
		}
		if !root && !model.IsPropagatedScope(d.Scope) {
			continue
		}
		dd := model.Dep{
			GroupID:    interp(d.GroupID, eff.Properties),
			ArtifactID: interp(d.ArtifactID, eff.Properties),
			Version:    interp(d.Version, eff.Properties),
			Scope:      d.Scope,
			Exclusions: d.Exclusions,
		}
		a := model.Artifact{GroupID: dd.GroupID, ArtifactID: dd.ArtifactID}
		if a.GroupID == "" || a.ArtifactID == "" {
			continue
		}
		if excludes[a] {
			continue
		}
		version := dd.Version
		if version == "" {
			if m, ok := eff.Managed[a]; ok {
				version = m.Version
			}
		}
		if version == "" || (strings.HasPrefix(version, "[") || strings.HasPrefix(version, "(")) {
			// Ranged or unversioned dependency; resolve conservatively.
			version = l.resolveRange(a, version)
		}
		if version == "" {
			// Could not resolve a concrete version -> mark artifact in-use
			// conservatively (protects all its versions).
			if inUse[a] == nil {
				inUse[a] = map[string]bool{}
			}
			inUse[a][model.KeepAllVersion] = true
			continue
		}
		coord := model.Coordinate{Artifact: a, Version: version}
		addVersion(inUse, coord)

		key := coord.String()
		if visited[key] {
			continue
		}
		visited[key] = true

		child, err := l.parseFromRepo(a.GroupID, a.ArtifactID, version)
		if err != nil {
			// Missing/corrupt POM: POM already treated in-use; stop descending.
			continue
		}
		childEff, err := l.effective(child, eff.Properties)
		if err != nil {
			continue
		}
		childEff.Managed = mergeManaged(eff.Managed, childEff.Managed)
		childExcludes := mergeExcludes(excludes, dd.Exclusions)
		l.expand(childEff, childExcludes, visited, inUse, false)
	}
}

func addVersion(set map[model.Artifact]map[string]bool, c model.Coordinate) {
	if c.Version == "" {
		return
	}
	if set[c.Artifact] == nil {
		set[c.Artifact] = map[string]bool{}
	}
	set[c.Artifact][c.Version] = true
}

func mergeExcludes(base map[model.Artifact]bool, exs []model.Exclusion) map[model.Artifact]bool {
	out := map[model.Artifact]bool{}
	for k, v := range base {
		out[k] = v
	}
	for _, e := range exs {
		out[model.Artifact{GroupID: e.GroupID, ArtifactID: e.ArtifactID}] = true
	}
	return out
}

func mergeManaged(base, more map[model.Artifact]model.ManagedDep) map[model.Artifact]model.ManagedDep {
	out := map[model.Artifact]model.ManagedDep{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range more {
		out[k] = v
	}
	return out
}

// resolveRange resolves a version-range or unversioned dependency to a concrete
// version present in the repo, or "" if none.
func (l *Loader) resolveRange(a model.Artifact, version string) string {
	if !(strings.HasPrefix(version, "[") || strings.HasPrefix(version, "(")) {
		return version
	}
	vers := l.availableVersions(a)
	if len(vers) == 0 {
		return ""
	}
	lo, hi, loIn, hiIn := parseRange(version)
	best := ""
	for _, cand := range vers {
		if inRange(cand, lo, hi, loIn, hiIn) {
			if best == "" || compareVersions(cand, best) > 0 {
				best = cand
			}
		}
	}
	return best
}

func (l *Loader) availableVersions(a model.Artifact) []string {
	groupPath := strings.ReplaceAll(a.GroupID, ".", "/")
	dir := filepath.Join(l.Repo, filepath.FromSlash(groupPath), a.ArtifactID)
	entries, err := readDirNames(dir)
	if err != nil {
		return nil
	}
	sort.Strings(entries)
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
	}
	return out
}

func parseRange(r string) (lo, hi string, loIn, hiIn bool) {
	t := strings.TrimSpace(r)
	loIn = strings.HasPrefix(t, "[")
	hiIn = strings.HasSuffix(t, "]")
	t = strings.TrimPrefix(t, "[")
	t = strings.TrimPrefix(t, "(")
	t = strings.TrimSuffix(t, "]")
	t = strings.TrimSuffix(t, ")")
	parts := strings.SplitN(t, ",", 2)
	if len(parts) < 2 {
		return "", "", loIn, hiIn
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), loIn, hiIn
}

func inRange(v, lo, hi string, loIn, hiIn bool) bool {
	if lo != "" {
		c := compareVersions(v, lo)
		if loIn && c < 0 {
			return false
		}
		if !loIn && c <= 0 {
			return false
		}
	}
	if hi != "" {
		c := compareVersions(v, hi)
		if hiIn && c > 0 {
			return false
		}
		if !hiIn && c >= 0 {
			return false
		}
	}
	return true
}
