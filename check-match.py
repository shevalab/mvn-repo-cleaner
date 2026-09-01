#!/usr/bin/env python3
"""Check that every artifact:version entry in an in-use list is actually
referenced by the POM files of the corresponding project folder.

The in-use list uses the format:
    === <project folder> ===
      <artifactId>:<version>
      ...

For each entry the script scans every pom.xml under <projects-root>/<folder>,
parses dependencies, dependencyManagement, plugins, pluginManagement and
profiles (with ${property} version resolution), and reports whether any POM
references the artifact with a (possibly property-resolved) matching version.

Section headers that do not name a real folder (e.g. a trailing
"=== summary ===" block) and summary/count lines are ignored.

Usage:
    check-match.py <in-use-list> [projects-root]
"""

import argparse
import os
import re
import sys
import xml.etree.ElementTree as ET

XMLNS_RE = re.compile(r"^\{[^}]*\}\s*")

LOCAL_NS_RE = re.compile(r"\{(.*?)\}([a-zA-Z0-9_.-]+)$")


def local_name(tag):
    return XMLNS_RE.sub("", tag or "")


def direct_children(el, name):
    return [c for c in el if local_name(c.tag) == name]


def find_child(el, name):
    for c in el:
        if local_name(c.tag) == name:
            return c
    return None


# Versions that are never literals. project.* / pom.* map onto the POM's own
# coordinates; env.* comes from the environment.
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


def collect(pom, props, coords):
    def add_one(dep):
        gid = find_child(dep, "groupId")
        aid = find_child(dep, "artifactId")
        ver = find_child(dep, "version")
        if aid is None or aid.text is None or not aid.text.strip():
            return
        artifact = aid.text.strip()
        version = ""
        if ver is not None and ver.text:
            version = resolver.resolve(ver.text.strip())
        coords.add((artifact, version))

    resolver = Resolver(props)

    def collect_block(el):
        if el is None:
            return
        for dep in direct_children(el, "dependency"):
            add_one(dep)

    deps = find_child(pom, "dependencies")
    collect_block(deps)

    dm = find_child(pom, "dependencyManagement")
    if dm is not None:
        collect_block(find_child(dm, "dependencies"))

    def collect_plugins(el):
        if el is None:
            return
        for plugin in direct_children(el, "plugin"):
            add_one(plugin)

    build = find_child(pom, "build")
    if build is not None:
        collect_plugins(find_child(build, "plugins"))
        pm = find_child(build, "pluginManagement")
        if pm is not None:
            collect_plugins(find_child(pm, "plugins"))


def scan_pom(path, coords, props):
    resolver = Resolver(props)
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

    collect(root, resolved_props, coords)

    profiles = find_child(root, "profiles")
    if profiles is not None:
        for profile in direct_children(profiles, "profile"):
            pprops = parse_properties(find_child(profile, "properties"))
            merged = dict(resolved_props)
            merged.update(pprops)
            collect(profile, merged, coords)


def parse_in_use_list(path):
    entries = []
    current = None
    with open(path, encoding="utf-8") as f:
        for raw in f:
            line = raw.rstrip("\n")
            m = re.match(r"\s*===\s*([^=]+?)\s*===\s*$", line)
            if m:
                current = m.group(1).strip()
                continue
            stripped = line.strip().lstrip("*").strip()
            stripped = re.sub(r"\s+\([^)]*\)\s*$", "", stripped).strip()
            if not stripped or ":" not in stripped:
                continue
            artifact, _, version = stripped.rpartition(":")
            if not artifact or not version:
                continue
            if any(ch.isspace() for ch in artifact):
                # summary/count lines (e.g. "stale entries: 1234") are not entries
                continue
            if current is None:
                continue
            entries.append((current, artifact.strip(), version.strip()))
    return entries


def match(entry, coords):
    folder, artifact, version = entry
    for cand_artifact, cand_version in coords:
        if cand_artifact != artifact:
            continue
        if cand_version == version:
            return True
        if version.startswith("${"):
            # The listed version itself is a property placeholder; a bare
            # (unversioned) reference is the closest verifiable match.
            if cand_version == "":
                return True
    return False


def main():
    parser = argparse.ArgumentParser(
        description="Verify in-use list entries against project folder POMs"
    )
    parser.add_argument("in_use_list", help="path to the in-use list file")
    parser.add_argument(
        "projects_root",
        nargs="?",
        default=os.path.expanduser("~/projects"),
        help="directory containing the project folders (default ~/projects)",
    )
    args = parser.parse_args()

    entries = parse_in_use_list(args.in_use_list)
    if not entries:
        print(f"no entries parsed from {args.in_use_list}")
        return 1

    missing = []
    skipped = 0
    checked = 0
    for folder, artifact, version in entries:
        folder_dir = os.path.join(args.projects_root, folder)
        if not os.path.isdir(folder_dir):
            # Section headers that are not real folders (e.g. "=== summary ===")
            # are ignored together with their entries.
            skipped += 1
            continue
        coords = set()
        for base, dirs, files in os.walk(folder_dir):
            dirs[:] = [d for d in dirs if d not in ("target", ".git", ".idea", ".gradle", ".m2")]
            if "pom.xml" in files:
                scan_pom(os.path.join(base, "pom.xml"), coords, {})
        if match((folder, artifact, version), coords):
            checked += 1
        else:
            missing.append((folder, artifact, version, "not found in POMs"))

    print(f"in-use entries: {len(entries)}")
    print(f"found in project POMs: {checked}")
    print(f"missing: {len(missing)}")
    if skipped:
        print(f"ignored (no such folder/header): {skipped}")
    if missing:
        print()
        print("=== missing ===")
        for folder, artifact, version, reason in missing:
            print(f"  {folder}  {artifact}:{version}  ({reason})")
    return 1 if missing else 0


if __name__ == "__main__":
    sys.exit(main())