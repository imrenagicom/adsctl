#!/usr/bin/env python3
"""Check the shared plugin and its contents in GoReleaser distributions."""

import argparse
import json
from pathlib import Path, PurePosixPath
import re
import tarfile
import zipfile


ROOT = Path(__file__).resolve().parent.parent
MANIFESTS = (".codex-plugin/plugin.json", ".claude-plugin/plugin.json")
CATALOGS = (".agents/plugins/marketplace.json", ".claude-plugin/marketplace.json")


def require(condition, message):
    if not condition:
        raise ValueError(message)


def check_source():
    codex, claude = [json.loads((ROOT / path).read_text()) for path in MANIFESTS]
    for key in ("name", "version", "description", "author", "repository"):
        require(codex.get(key) == claude.get(key) and codex.get(key),
                f"Plugin manifests must have matching {key}")
    require(codex["name"] == "adsctl", "Unexpected plugin identity")
    require(re.fullmatch(r"\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", codex["version"]),
            "Plugin version must be semantic")
    require(codex.get("skills") == "./skills/", "Codex must load the shared skills")
    require(codex.get("requires_local_executor") is True,
            "The CLI plugin needs a local executor")
    onboarding = codex["extensions"]["com.openai"]["onboardingSkill"]
    require(onboarding == "./skills/adsctl-setup/SKILL.md" and (ROOT / onboarding).is_file(),
            "Onboarding must point to the packaged setup skill")
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
    print(f"Plugin {codex['name']} {codex['version']}: both catalogs and skills checked")


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
    required = [*MANIFESTS, *CATALOGS, "LICENSE", "README.md", "docs/plugins.md", "install.sh"]
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
    args = parser.parse_args()
    check_source()
    if args.archives:
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
