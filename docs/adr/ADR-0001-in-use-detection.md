# ADR-0001: In-use detection scope and safety model

Status: Accepted

## Context

`mvn-repo-cleaner` deletes dependencies from a local Maven repo (`~/.m2`). The
core design questions were: (1) how to determine what is "in use", (2) how
precisely to delete, and (3) how to keep the tool safe on a destructive
operation.

## Decision

1. **Full transitive tracking.** A dependency is in use if it appears in the
   full transitive closure of any scanned project, not just as a direct
   declaration. POM resolution follows parent POMs, `dependencyManagement`
   (incl. BOM import), property interpolation, and version ranges.

2. **Version-granular deletion, union across projects.** Delete at the version
   level. If any project uses a version of an artifact, that version is kept;
   all projects' used versions are unioned, so different projects using
   different versions of the same artifact keep all of them.

3. **Conservative on uncertainty.** Missing/corrupt POMs and unresolvable
   versions are treated as **in use**, so they are never deleted.

4. **Build system:** Maven `pom.xml` only (v1). `~/.m2` is Maven-specific;
   Gradle support is out of scope for now.

5. **Safety model:** dry-run by default; `--confirm` performs actual deletion.
   `--export` is export-only and refuses combination with deletion flags.

6. **List-driven operation:** `--export <file>` writes one absolute path per
   line; `--from-file <file>` deletes exactly the listed paths without scanning.

## Status

This ADR captures the decisions from the initial design session (see
`docs/PLAN.md`). Subsequent ADRs will record changes as the tool evolves.
