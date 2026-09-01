# check-match scripts

The `check-match*.py` scripts cross-check coordinate lists against the actual
`pom.xml` content of the project folders. They are analysis/audit helpers for
the mvn-repo-cleaner workflow, not part of the Go tool.

Both scripts scan every `pom.xml` under a project root (default `~/projects`)
and understand:

- `<dependencies>` and `<dependencyManagement>`
- `<build><plugins>` and `<build><pluginManagement><plugins>`
  (plugins without a `<groupId>` default to `org.apache.maven.plugins`)
- `<profiles>` (dependencies, dependencyManagement, plugins, properties)
- `${property}` version/groupId interpolation, including `project.*` / `pom.*`
  built-ins and the POM's own declared properties

## check-match.py — in-use list vs. project folders

Verifies that every `artifactId:version` entry in an in-use list is actually
referenced by a POM in the corresponding project folder.

### Usage

```
python3 check-match.py <in-use-list> [projects-root]
```

- `in-use-list`  input file in the group format below (required).
- `projects-root` folder that contains the project directories
  (default `~/projects`).

### In-use list format

```
=== <project folder> ===
  <artifactId>:<version>
  ...
```

A section header `=== name ===` names a subdirectory of `projects-root`.
Indented `artifactId:version` lines below it are the entries to verify.

### Behavior

For each entry `(folder, artifact, version)` the script locates
`projects-root/folder`, parses every POM under it, and checks whether any POM
references the artifact with a (possibly property-resolved) matching version.
An entry whose listed version is itself a property placeholder (e.g.
`${main.version}`) is considered a match when the artifact is referenced
without an explicit version.

Section headers that do not name a real folder (e.g. a trailing
`=== summary ===` block) and summary/count lines are ignored.

### Output

```
in-use entries: 269
found in project POMs: 269
missing: 0
```

`missing` entries are listed individually. Entries in sections without a real
folder are reported as `ignored`. Exit code is `1` when any entry is missing,
`0` otherwise.

The parser also tolerates a trailing `(note)` suffix on entry lines, so the
output of `check-match-stale.py` can be fed back in unchanged.

Example:

```sh
python3 check-match.py in-use-list.txt
```

## check-match-stale.py — stale list vs. project folders

Verifies that every path in a stale list is *genuinely* stale, i.e. is **not**
referenced by any POM in the project folders. Entries that are referenced are
false positives that should be removed from the stale list.

### Usage

```
python3 check-match-stale.py <stale-file> [projects-root] [--repo REPO]
```

- `stale-file`  one absolute Maven repository path per line (required).
- `projects-root` folder that contains the project directories
  (default `~/projects`).
- `--repo REPO` local Maven repository root to strip from the paths so the
  group path can be reconstructed (default `~/.m2/repository`). If a path does
  not start with the repo prefix, the script falls back to the last
  `/repository/` boundary, or treats the path as repo-relative.

### Stale list format

```
<repo>/<group/path>/<artifact>/<version>
```

The artifact is the second-to-last path segment and the version the last one;
everything before them becomes the (dot-joined) groupId.

### Behavior

The script scans all POMs under `projects-root` and collects every
`groupId:artifactId:version` reference per project folder. Each stale entry is
then classified:

- **no reference found** — genuinely stale, keep in the stale list.
- **full-coordinate match** — the exact `group:artifact:version` is referenced;
  a false positive, remove from the stale list.
- **artifact:version match with a different group** — only the artifact and
  version match, but the on-disk group differs from the referenced group
  (e.g. `com.vmware:vijava` vs. the in-use `com.cloudbees.thirdparty:vijava`);
  reported with the found group(s) for manual review.

### Output

The output uses the in-use-list.txt layout (grouped by project folder), so it
can be fed back into `check-match.py`. It starts directly with sections:

```
=== intigua-ms.utils ===
  httpcomponents-client:4.4
  vijava:5.1  (different group: found com.cloudbees.thirdparty)

=== summary ===
  stale entries: 5421
  genuinely stale (no reference found): 5407
  referenced in use (mismatch): 14
  full-coordinate matches: 10
  artifact:version matches (different group): 4
```

Each `=== <folder> ===` section lists the stale entries referenced by that
folder as `artifactId:version`; entries whose group differs from the referenced
group carry an inline `(different group: ...)` note. A final
`=== summary ===` section holds the totals.

Exit code is `1` when any entry is referenced, `0` otherwise.

Example:

```sh
python3 check-match-stale.py stale.txt
```

## Notes

- Both scripts are read-only: they never modify POMs or the repository.
- Scripts are self-contained and share no imports; each may be copied or run
  standalone.
- `check-match-stale.py` output round-trips directly into `check-match.py`:
  the reference analysis (`in-use-list.txt`) and the stale check consistently
  agree on which coordinates are in use.