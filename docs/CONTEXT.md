# Domain Context — mvn-repo-cleaner

This file records the shared vocabulary and domain model for the project. It is
the source of truth for terminology used across the codebase and docs.

## Glossary

| Term     | Definition                                                                 |
|----------|----------------------------------------------------------------------------|
| **Artifact** | A `groupId:artifactId` coordinate (e.g. `org.slf4j:slf4j-api`). |
| **Version**  | A specific version of an artifact (`1.0`, incl. qualifiers/SNAPSHOTs). |
| **Coordinate** | A fully-resolved artifact + version (`groupId:artifactId:version`). |
| **In Use** | An artifact-version reachable from any scanned project's full transitive closure. |
| **Stale**  | Present on disk under the repo root, but not in the in-use set. |
| **Repo**   | The deletion-scope root on disk (default `~/.m2/repository`). |
| **POM**    | A Maven project object model file (`pom.xml`); the unit we parse. |
| **Scope**  | A dependency scope (`compile`, `runtime`, `test`, ...). Only `compile` and `runtime` count as in-use for retention. |

## Maven repository layout

```
<repo>/<group/path>/<artifact>/<version>/
    <artifact>-<version>.pom
    <artifact>-<version>.jar
    ...
```

Group dots become path segments (`com.example` → `com/example`). A directory is
identified as a *version directory* by containing a file prefixed
`<artifact>-<version>.`.

## Retention rule

An artifact-version is kept if and only if it is in the in-use set. Different
projects may use different versions of the same artifact; the in-use set is the
**union** across all scanned projects, so all used versions are kept.

When a dependency's version cannot be resolved to something verifiable (missing
POM, unresolvable range), the artifact is treated as **in use** conservatively —
all its versions are kept.
