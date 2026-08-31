# mvn-repo-cleaner — Design & Implementation Plan

## Description (from idea.md)

Console utility for cleaning stale dependencies from a local Maven repository
(`~/.m2`). Java project folders are scanned recursively and all components in
the local Maven repository that are not in use are deleted.

## Goal

A GoLang console utility that:

1. Scans Java/Maven projects (recursively finding `pom.xml`).
2. Builds the complete *in-use* dependency set (direct + transitive).
3. Compares it against what exists on disk in the local repo.
4. Reports / exports / deletes anything not in use — safely.

---

## Domain Model

| Term     | Definition                                              |
|----------|---------------------------------------------------------|
| Artifact | `groupId:artifactId` (e.g. `org.slf4j:slf4j-api`)       |
| Version  | A specific version of an artifact (`1.0`, incl. qualifiers/SNAPSHOT) |
| In Use   | An artifact-version reachable from any scanned project's full transitive closure |
| Stale    | On disk in the repo, but not in the in-use set          |
| Repo     | Deletion-scope root (default `~/.m2/repository`, overridable) |

---

## Locked Decisions (from grilling session)

1. **Transitive tracking** — track the full transitive closure, not just direct
   dependencies.
2. **Build system** — Maven `pom.xml` parsing only (no Gradle in v1).
3. **Deletion granularity** — delete at the *version* level; keep only the
   specific versions in use.
4. **Deletion scope** — configurable via `--repo` flag, default `~/.m2/repository`.
5. **Safety model** — dry-run by default; `--confirm` actually deletes.
6. **Project input** — positional args for project paths.
7. **List I/O** — flag to export detected-stale list to a file; flag to remove
   files listed in a (possibly hand-edited) existing file.
8. **Unverifiable POMs** — if a transitive POM is missing/corrupt, skip it and
   treat that artifact as *in use* (conservative — never delete what we couldn't
   verify).
9. **Output format** — plain text lines.

### Post-plan refinements

- **List file format**: one absolute path per line.
- **`--export` behavior**: export-only; if `--export` is given it refuses any
  deletion/conflict (e.g. `--confirm` combined with `--export` is an error).
- **Missing root artifact**: a scanned project's own artifact, if missing from
  repo, is counted as in-use (conservative).

### Version-granularity across projects (Q10)

Different projects may use different versions of the same artifact
(`A` uses `foo:bar:1.0`, `B` uses `foo:bar:2.0`). Both are in use → both kept.
The in-use set is the *union* of all projects' resolved versions.

---

## CLI

```
mvn-repo-cleaner [paths...]                 # dry-run: scan, print stale list
  --confirm                                 # actually delete after scanning
  --repo <dir>                              # deletion scope; default ~/.m2/repository
  --export <file>                           # write stale list to file (plain text), no deletion
  --from-file <file>                        # delete exactly what's in the file (no scan)
```

Mapping to the three original modes from idea.md:

| idea.md mode        | CLI form                              |
|---------------------|---------------------------------------|
| list only           | plain run, or `--export <file>`       |
| delete with a list  | `--from-file <file>`                  |
| scan and delete     | scan run with `--confirm`             |

### Flag rules

- `--from-file` = standalone mode: **no scan**; the file fully defines what to
  delete. Reading that list, walk the repo, delete listed paths (with
  `--confirm`; dry-run otherwise).
- `--export` = export-only: conflicts with `--confirm` / `--from-file` are
  rejected.
- `--confirm` only meaningful in scan mode or `--from-file` mode.

---

## Architecture / Package Layout

```
cmd/mvn-repo-cleaner/main.go      # arg parsing, orchestration, output
internal/model/                   # domain types: Artifact, Coordinate, POM, Dep, ...
internal/resolve/pom.go           # POM XML parsing
internal/resolve/expand.go        # transitive closure (property interp, parent, depMgmt, ranges)
internal/scan/scan.go             # find pom.xml recursively under project paths
internal/scan/useset.go           # merge project in-use sets
internal/repo/walk.go             # enumerate existing artifacts/versions on disk
internal/repo/delete.go           # deletion logic (per version, from set)
internal/repo/export.go           # write/read plain-text list
```

---

## Implementation Phases

### Phase 1 — Scaffolding
- Init Go module.
- `main.go` + flag parsing + validation of flag combinations.
- gofmt / go vet clean.

### Phase 2 — Repo enumeration + list I/O (no resolution yet)
- `repo.Walk`: parse the `.m2/repository` path/version directory structure into
  `artifact -> [versions]`.
- `export.Write` / `export.Read`: plain-text lines (one absolute path per line).
- Implement `--from-file` deletion against the walked repo.

### Phase 3 — POM parsing
- Parse `pom.xml` into a struct: `groupId/artifactId/version`, `parent`,
  `properties`, `dependencies`, `dependencyManagement`, `modules`, `packaging`.
- Property interpolation `${...}` from local POM properties + parent inheritance.

### Phase 4 — Transitive resolution
Faithful subset of Maven resolution:
1. Given a root POM, produce resolved dependency set (direct + transitives).
2. Follow parent POM inheritance (read parent from repo).
3. Apply `dependencyManagement` (BOM import scope + in-POM version pinning).
4. Support version ranges (`[1.0,2.0)`) → resolve to a concrete version found in
   repo.
5. Exclusions (`<exclusions>`), scope filtering.
6. **Cycle detection** — track visited artifacts; don't recurse infinitely.
7. Missing/corrupt POM or unresolvable → treat that artifact as **in use**
   (conservative), continue.
8. Nearest-version conflict resolution when the same artifact is reached by
   multiple paths.

### Phase 5 — Scan + in-use set merge
- Under each project path, recursively find `pom.xml`.
- For each, produce resolved transitive set.
- Merge into global in-use set: artifact -> set of versions (union).
- Multi-module reactor handled naturally by recursive pom discovery.

### Phase 6 — Diff + deletion
- Compare repo-enumerated versions vs in-use set.
- Stale = in repo but not in use → print/export.
- `--confirm`: delete stale version directories.
- Summary report: kept / stale / deleted / skipped-unverifiable counts.

### Phase 7 — Tests
- Unit: POM parser, property interpolation, dependencyManagement, ranges,
  exclusions, cycle handling.
- Integration (temp fake repo + temp fake project): full scan → in-use → diff →
  delete; verify correct dirs removed/kept.
- Golden-file test for export/import round-trip.

### Phase 8 — Docs
- README: usage, flags, safety model, list-file format spec.
- `CONTEXT.md` and ADR recording the locked decisions.
