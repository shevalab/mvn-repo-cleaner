#!/usr/bin/env python3
"""Check that every path in a stale list is genuinely stale, i.e. is NOT
referenced by any POM under the project folders.

The stale list contains one absolute Maven repository path per line:
    <repo>/<group/path>/<artifact>/<version>
The artifact is the second-to-last path segment and the version the last one.

Every "group:artifact:version" reference is extracted from all pom.xml files
under <projects-root> (dependencies, dependencyManagement, plugins,
pluginManagement and profiles, with ${property} version resolution). An entry
is reported as "in use" (a false-positive stale entry) when a reference matches
it -- by full coordinates when possible, by artifactId:version otherwise.

The output follows the in-use-list.txt layout: one "=== <folder> ===" section
per project folder listing the referenced stale entries it owns, then a
"=== summary ===" section with the totals.

Usage:
    check-match-stale.py <stale-file> [projects-root] [--repo REPO]
"""

import argparse
import os
import re
import sys
import xml.etree.ElementTree as ET

XMLNS_RE = re.compile(r"^\{[^}]*\}\s*")


def local_name(tag):
    return XMLNS_RE.sub("", tag or "")


def direct_children(el, name):
    return [c for c in el if local_name(c.tag) == name]


def find_child(el, name):
    for c in el:
        if local_name(c.tag) == name:
            return c
    return None


BUILTIN_PROPS = {
    "project.groupId",
    "project.artifactId",
    "project.version",
    "pom.version",
}


class Resolver:
    def __init__(self, props):
        self.props = dict(props)
        self.cache = {}
        self.seen = set()

    def resolve(self, value):
        if not value:
            return value
        if value in self.cache:
            return self.cache[value]
        if value in self.seen:
            return value
        self.seen.add(value)
        out = re.sub(
            r"\$\{([^}]+)\}",
            lambda m: self.lookup(m.group(1)),
            value,
        )
        self.seen.discard(value)
        self.cache[value] = out
        return out

    def lookup(self, key):
        if key in self.props:
            return self.resolve(self.props[key])
        if key in BUILTIN_PROPS:
            return ""
        if key.startswith("env."):
            return os.environ.get(key[4:], "${%s}" % key)
        return "${%s}" % key


def parse_properties(el):
    props = {}
    if el is None:
        return props
    for c in el:
        if c.tag is not None:
            props[local_name(c.tag)] = c.text or ""
    return props


DEFAULT_PLUGIN_GROUP = "org.apache.maven.plugins"


def add_ref(refs, dep, resolver, is_plugin=False):
    gid = find_child(dep, "groupId")
    aid = find_child(dep, "artifactId")
    ver = find_child(dep, "version")
    if aid is None or aid.text is None or not aid.text.strip():
        return
    group = ""
    if gid is not None and gid.text:
        group = resolver.resolve(gid.text.strip())
    if not group and is_plugin:
        group = DEFAULT_PLUGIN_GROUP
    artifact = aid.text.strip()
    version = ""
    if ver is not None and ver.text:
        version = resolver.resolve(ver.text.strip())
    refs.add((group, artifact, version))


def scan_pom(path, refs, props):
    try:
        tree = ET.parse(path)
    except (ET.ParseError, OSError):
        return
    root = tree.getroot()

    own_props = parse_properties(find_child(root, "properties"))
    resolved_props = dict(props)
    resolved_props.update(own_props)

    own_gid = find_child(root, "groupId")
    own_aid = find_child(root, "artifactId")
    own_ver = find_child(root, "version")
    if own_gid is not None and own_gid.text:
        resolved_props["project.groupId"] = own_gid.text.strip()
    if own_aid is not None and own_aid.text:
        resolved_props["project.artifactId"] = own_aid.text.strip()
    if own_ver is not None and own_ver.text:
        resolved_props["project.version"] = own_ver.text.strip()
        resolved_props["pom.version"] = own_ver.text.strip()
    resolver = Resolver(resolved_props)

    def collect_block(el):
        if el is None:
            return
        for dep in direct_children(el, "dependency"):
            add_ref(refs, dep, resolver)

    collect_block(find_child(root, "dependencies"))
    dm = find_child(root, "dependencyManagement")
    if dm is not None:
        collect_block(find_child(dm, "dependencies"))

    def collect_plugins(el):
        if el is None:
            return
        for plugin in direct_children(el, "plugin"):
            add_ref(refs, plugin, resolver, is_plugin=True)

    build = find_child(root, "build")
    if build is not None:
        collect_plugins(find_child(build, "plugins"))
        pm = find_child(build, "pluginManagement")
        if pm is not None:
            collect_plugins(find_child(pm, "plugins"))

    profiles = find_child(root, "profiles")
    if profiles is not None:
        for profile in direct_children(profiles, "profile"):
            pprops = parse_properties(find_child(profile, "properties"))
            merged = dict(resolved_props)
            merged.update(pprops)
            for dep in direct_children(profile, "dependencies"):
                for d in direct_children(dep, "dependency"):
                    add_ref(refs, d, Resolver(merged))


def split_repo_path(path, repo):
    p = path.replace("\\", "/")
    if repo:
        r = repo.replace("\\", "/").rstrip("/")
        if p == r:
            return ""
        if p.startswith(r + "/"):
            return p[len(r) + 1 :]
    # Best effort without a repo argument: drop everything up to the last
    # "/repository/" boundary (the standard Maven repo layout marker).
    idx = p.rfind("/repository/")
    if idx != -1:
        return p[idx + len("/repository/") :]
    return p.lstrip("/")


def parse_stale_file(path, repo):
    entries = []
    with open(path, encoding="utf-8") as f:
        for raw in f:
            line = raw.strip()
            if not line:
                continue
            parts = split_repo_path(line, repo).split("/")
            if len(parts) < 2:
                continue
            artifact = parts[-2]
            version = parts[-1]
            group = ".".join(parts[:-2])
            entries.append((line, group, artifact, version))
    return entries


def match_refs(group, artifact, version, refs):
    exact = set()
    artifact_version = set()
    for (rg, ra, rv) in refs:
        if ra != artifact:
            continue
        if version and rv == version:
            artifact_version.add(rg)
            if rg == group:
                exact.add(rg)
    return exact, artifact_version


def main():
    parser = argparse.ArgumentParser(
        description="Verify stale list entries are genuinely stale (not referenced in project POMs)"
    )
    parser.add_argument("stale_file", help="path to the stale list file")
    parser.add_argument(
        "projects_root",
        nargs="?",
        default=os.path.expanduser("~/projects"),
        help="directory containing the project folders (default ~/projects)",
    )
    parser.add_argument(
        "--repo",
        default=os.path.expanduser("~/.m2/repository"),
        help="local maven repository root to strip from paths (default ~/.m2/repository)",
    )
    args = parser.parse_args()

    entries = parse_stale_file(args.stale_file, args.repo)
    if not entries:
        print(f"no entries parsed from {args.stale_file}")
        return 1

    refs_by_folder = {}
    for folder in os.listdir(args.projects_root):
        folder_dir = os.path.join(args.projects_root, folder)
        if not os.path.isdir(folder_dir):
            continue
        folder_refs = set()
        for base, dirs, files in os.walk(folder_dir):
            dirs[:] = [d for d in dirs if d not in ("target", ".git", ".idea", ".gradle", ".m2")]
            if "pom.xml" in files:
                scan_pom(os.path.join(base, "pom.xml"), folder_refs, {})
        if folder_refs:
            refs_by_folder[folder] = folder_refs

    sections = []
    exact_matched = set()
    artifact_matched = set()
    for folder, folder_refs in sorted(refs_by_folder.items()):
        hits = []
        for idx, (_, group, artifact, version) in enumerate(entries):
            exact, artifact_version = match_refs(group, artifact, version, folder_refs)
            if exact:
                hits.append((artifact, version, ""))
                exact_matched.add(idx)
            elif artifact_version:
                groups = ", ".join(sorted(artifact_version))
                hits.append((artifact, version, "different group: found " + groups))
                artifact_matched.add(idx)
        hits.sort(key=lambda h: h[0].lower())
        if hits:
            sections.append((folder, hits))

    matched = exact_matched | artifact_matched
    in_use = len(matched)
    stale = len(entries) - in_use

    for folder, hits in sections:
        print(f"=== {folder} ===")
        for artifact, version, note in hits:
            coord = f"{artifact}:{version}" if version else artifact
            if note:
                print(f"  {coord}  ({note})")
            else:
                print(f"  {coord}")
        print()

    print("=== summary ===")
    print(f"  stale entries: {len(entries)}")
    print(f"  genuinely stale (no reference found): {stale}")
    print(f"  referenced in use (mismatch): {in_use}")
    print(f"  full-coordinate matches: {len(exact_matched)}")
    print(f"  artifact:version matches (different group): {len(artifact_matched)}")

    return 1 if in_use else 0


if __name__ == "__main__":
    sys.exit(main())