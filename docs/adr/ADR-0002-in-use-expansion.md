# ADR-0002: Treat all project-declared coordinates as in-use

Status: Accepted

## Context

`mvn-repo-cleaner` reported some components as stale even though a scanned Java
project referenced them. The stale detector only kept artifacts that were
*actually reached* as live dependencies during transitive expansion. Components
that a project explicitly declares — but that are not resolved onto the runtime
classpath as ordinary `<dependency>` entries — were wrongly considered stale.

Concretely, these resulted in false-positive stale entries:

- **`maven-dependency-plugin` `<artifactItem>` references.** Zip/web resources
  (e.g. `angular-web-client`, `dashboard`, `patch-web-client`) are unpacked into
  the WAR by the plugin's `unpack` goal. They appear in plugin
  `<configuration><artifactItems>` but are not `<dependency>` declarations.
- **`dependencyManagement`-only entries.** Coordinates pinned in a BOM or in a
  project's own `dependencyManagement` (e.g. `bcprov-jdk18on`, `swagger-models`,
  `swagger-springmvc`, `httpcomponents-client`, `jwt-helper`) were not kept unless
  some module depended on them directly.
- **Profile references.** Coordinates declared only inside `<profiles>` were not
  scanned at all.
- **Imported-BOM precedence.** A project's own `dependencyManagement` was applied
  *before* imported BOMs, so an imported BOM's managed versions unintentionally
  overwrote the project's own pins (e.g. `jwt-provider`).

The audit scripts (`check-match.py` / `check-match-stale.py`) already treated
dependencies, `dependencyManagement`, plugins, `pluginManagement`, profiles and
plugin `artifactItem`s as "in use by a java project". The Go tool disagreed.

## Decision

Expand the Go resolver's in-use detection to match the audit scripts, staying
conservative:

1. **Every `dependencyManagement` entry** of a scanned project is in-use, even if
   never reached as a live dependency. Unresolvable versions (e.g. a `${\...}`
   whose property cannot be resolved) are treated as in-use *for every version*
   (the existing keep-all sentinel).
2. **Imported-BOM precedence is corrected.** A project's own
   `dependencyManagement` is applied after imported BOMs and inherited parents,
   so project entries override them (Maven semantics).
3. **Plugin `configuration` `artifactItem` references** are recorded as in-use,
   covering the `maven-dependency-plugin` unpack use case.
4. **Profile references** (dependencies, dependencyManagement and plugins inside
   `<profiles>`) are recorded as in-use regardless of activation.

## Consequences

- Components a project explicitly declares are no longer reported stale.
- The Go tool and the audit scripts now agree on what "in use" means.
- Behavior is more conservative: any managed or profile-declared coordinate is
  retained even when orphaned. This trades some cleanup thoroughness for safety,
  which is consistent with the project's conservative-by-design model
  (ADR-0001).
- Correcting BOM precedence can change which versions are retained wherever a
  project overrides an imported BOM.
