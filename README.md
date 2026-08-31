# mvn-repo-cleaner

A GoLang console utility to clean stale dependencies from a local Maven
repository (`~/.m2`). It scans Java/Maven projects recursively, builds the full
**in-use** dependency set (direct + transitive), and deletes anything on disk
that is not in use — safely (dry-run by default).

## How it works

1. Scan each given project path recursively for `pom.xml` files.
2. For each POM, resolve its **complete** dependency graph (direct + transitive,
   following parent POMs, `dependencyManagement`, property interpolation, and
   version ranges).
3. Merge all projects' in-use sets. Different projects using different versions
   of the same artifact keep **all** of those versions.
4. Compare the in-use set against what actually exists under the repo root on
   disk. Anything present but not in use is **stale**.
5. Report (dry-run) or delete (with `--confirm`) the stale paths.

### Conservative by design

- Missing/corrupt POMs or unresolvable versions are treated as **in use** —
  we never delete something we couldn't verify.
- Deletion is **dry-run by default**; pass `--confirm` to actually delete.

## Usage

```
mvn-repo-cleaner [options] [project-paths...]
```

Examples:

```sh
# Dry-run: scan ./projects and print what would be deleted
mvn-repo-cleaner ./projects

# Delete stale dependencies (after reviewing the dry-run output)
mvn-repo-cleaner --confirm ./projects

# Point at a non-default repo
mvn-repo-cleaner --repo /custom/m2/repository ./projects

# Export the stale list to a file (no deletion)
mvn-repo-cleaner --export stale.txt ./projects

# Delete exactly what's listed in a file (possibly hand-edited; no scan)
mvn-repo-cleaner --confirm --from-file stale.txt
```

## Flags

| Flag            | Description                                                        |
|-----------------|--------------------------------------------------------------------|
| `--confirm`     | Actually delete. Without it, only a dry-run list is printed.       |
| `--repo <dir>`  | Repository root to inspect/delete from. Default `~/.m2/repository`.|
| `--export <file>` | Write the computed stale list to `<file>` (export-only; conflicts with `--confirm`/`--from-file`). |
| `--from-file <file>` | Delete exactly the paths in `<file>`; no scan is performed.    |

## List file format

Plain text, **one absolute path per line** (as produced by `--export`). Lines
are trimmed; empty lines are ignored. This is the format expected by
`--from-file`, so you can export, hand-edit, and re-import a list.

## Development

```sh
go build ./... && go vet ./...
go test ./...
```

See `docs/PLAN.md` for the design and implementation plan, and `docs/adr/` for
recorded architectural decisions.
