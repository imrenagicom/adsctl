#!/usr/bin/env python3
"""Check the shared plugin and its contents in GoReleaser distributions."""

import argparse
import json
from pathlib import Path, PurePosixPath
import re
import subprocess
import tarfile
import zipfile


ROOT = Path(__file__).resolve().parent.parent
MANIFESTS = (".codex-plugin/plugin.json", ".claude-plugin/plugin.json", "plugin.json")
CATALOGS = (".agents/plugins/marketplace.json", ".claude-plugin/marketplace.json")
SKILLS_CONFIG = ".agents/skills.json"
VERSION_PATTERN = r"\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?"


def require(condition, message):
    if not condition:
        raise ValueError(message)


def normalize_version(version):
    cleaned = version[1:] if version.startswith("v") else version
    require(re.fullmatch(VERSION_PATTERN, cleaned),
            f"Plugin version must be semantic: {version}")
    return cleaned


def latest_git_version():
    result = subprocess.run(
        ["git", "describe", "--tags", "--abbrev=0", "--match", "v[0-9]*"],
        cwd=ROOT,
        text=True,
        capture_output=True,
        check=False,
    )
    if result.returncode != 0:
        return None
    tag = result.stdout.strip()
    return normalize_version(tag) if tag else None


def set_version(version):
    normalized = normalize_version(version)
    for path in MANIFESTS:
        manifest_path = ROOT / path
        text = manifest_path.read_text()
        updated, count = re.subn(
            r'("version"\s*:\s*)"[^"]+"',
            rf'\g<1>"{normalized}"',
            text,
            count=1,
        )
        require(count == 1, f"Missing version field in {path}")
        require(json.loads(updated).get("version") == normalized,
                f"Failed to set version in {path}")
        manifest_path.write_text(updated)
    print(f"Updated plugin manifests to version {normalized}")
    return normalized


def check_source(expected_version=None):
    codex, claude, antigravity = [json.loads((ROOT / path).read_text()) for path in MANIFESTS]
    for key in ("name", "version", "description"):
        require(codex.get(key) == claude.get(key) == antigravity.get(key) and codex.get(key),
                f"Plugin manifests must have matching {key}")
    for key in ("author", "repository"):
        require(codex.get(key) == claude.get(key) and codex.get(key),
                f"Codex and Claude manifests must have matching {key}")
    require(codex["name"] == "adsctl", "Unexpected plugin identity")
    require(re.fullmatch(VERSION_PATTERN, codex["version"]),
            "Plugin version must be semantic")
    target_version = normalize_version(expected_version) if expected_version else latest_git_version()
    if target_version is not None:
        require(codex["version"] == target_version,
                f"Plugin version {codex['version']} must match release version {target_version}")
    require(antigravity.get("$schema") == "https://antigravity.google/schemas/v1/plugin.json",
            "Antigravity manifest must declare the v1 plugin schema")
    require(antigravity.get("displayName") == codex["interface"]["displayName"],
            "Antigravity displayName must match Codex")
    require(antigravity.get("suggestedPrompts") == codex["interface"]["defaultPrompt"],
            "Antigravity suggestedPrompts must match Codex")
    require(codex.get("skills") == "./skills/", "Codex must load the shared skills")
    require(codex.get("requires_local_executor") is True,
            "The CLI plugin needs a local executor")
    onboarding = codex["extensions"]["com.openai"]["onboardingSkill"]
    require(onboarding == "./skills/adsctl-setup/SKILL.md" and (ROOT / onboarding).is_file(),
            "Onboarding must point to the packaged setup skill")
    skills_config = json.loads((ROOT / SKILLS_CONFIG).read_text())
    require(skills_config == {"entries": [{"path": "skills"}]},
            "Workspace skills.json must point to ./skills")
    skills = {p.parent.name for p in (ROOT / "skills").glob("*/SKILL.md")}
    require(skills == {"adsctl", "adsctl-setup", "adsctl-meta-analyst"},
            "All shared skills must be discoverable")
    for path in CATALOGS:
        catalog = json.loads((ROOT / path).read_text())
        require(catalog["name"] == "adsctl", f"Wrong marketplace name in {path}")
        require(len(catalog["plugins"]) == 1, f"Expected one plugin in {path}")
        entry = catalog["plugins"][0]
        require(entry["name"] == codex["name"], f"Wrong plugin name in {path}")
        source = entry["source"]
        require(source == "./" or source == {"source": "local", "path": "./"},
                f"Marketplace source must resolve to the package root in {path}")
    for doc in (ROOT / "skills").rglob("*.md"):
        for link in re.findall(r"\]\(([^)]+)\)", doc.read_text()):
            if "://" not in link and not link.startswith("#"):
                target = (doc.parent / link.split("#")[0]).resolve()
                require(target.is_relative_to(ROOT / "skills") and target.is_file(),
                        f"Missing or external skill reference in {doc}: {link}")
    print(f"Plugin {codex['name']} {codex['version']}: manifests, catalogs, and skills checked")
    return codex["version"]


def check_archive(path):
    if path.suffix == ".zip":
        with zipfile.ZipFile(path) as archive:
            files = {name: archive.read(name) for name in archive.namelist()
                     if not name.endswith("/")}
    else:
        with tarfile.open(path) as archive:
            files = {entry.name: archive.extractfile(entry).read()
                     for entry in archive.getmembers() if entry.isfile()}
    # GoReleaser can place all files at the archive root or under one directory.
    roots = [name[:-len(MANIFESTS[0])] for name in files if name.endswith(MANIFESTS[0])]
    require(len(roots) == 1, f"Expected one plugin root in {path.name}")
    prefix = roots[0]
    required = [*MANIFESTS, *CATALOGS, SKILLS_CONFIG, "LICENSE", "README.md", "docs/plugins.md", "install.sh"]
    required.extend(p.relative_to(ROOT).as_posix()
                    for p in (ROOT / "skills").rglob("*") if p.is_file())
    for relative in required:
        require(files.get(prefix + relative) == (ROOT / relative).read_bytes(),
                f"Missing or changed plugin file in {path.name}: {relative}")
    binary = "adsctl.exe" if "_windows_" in path.name else "adsctl"
    require(prefix + binary in files, f"Missing CLI binary in {path.name}")
    require(all(not PurePosixPath(name).is_absolute() and ".." not in PurePosixPath(name).parts
                for name in files), f"Unsafe archive paths in {path.name}")
    print(f"{path.name}: CLI, manifests, catalogs, and all skill resources present")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archives", type=Path, help="Also verify all six distribution archives")
    parser.add_argument("--version", help="Expected semantic release version (e.g. 0.7.0 or v0.7.0)")
    parser.add_argument("--set-version", help="Update all plugin manifests to the given semantic version")
    args = parser.parse_args()
    if args.set_version:
        set_version(args.set_version)
    expected_version = args.version or args.set_version
    plugin_version = check_source(expected_version=expected_version)
    if args.archives:
        metadata_path = args.archives / "metadata.json"
        if metadata_path.is_file():
            metadata = json.loads(metadata_path.read_text())
            archive_version = normalize_version(metadata["tag"])
            require(plugin_version == archive_version,
                    f"Plugin version {plugin_version} must match archive tag version {archive_version}")
        archives = sorted([*args.archives.glob("adsctl_*.tar.gz"),
                           *args.archives.glob("adsctl_*.zip")])
        require(len(archives) == 6, "Expected six OS/architecture archives")
        for archive in archives:
            check_archive(archive)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError) as error:
        raise SystemExit(f"Plugin check failed: {error}") from error
