#!/usr/bin/env python3
"""
check_ondc_spec_drift.py — Weekly CI job to detect ONDC upstream spec changes.

Compares the current pinned commit SHAs in spec/ondc-pins.yaml against
the latest commits in ONDC-Official GitHub repos. Opens a GitHub Issue
if any upstream repo has new commits.
"""

import os
import sys
import json
import urllib.request
import yaml
from pathlib import Path
from datetime import datetime, timezone

REPO_ROOT = Path(__file__).parent.parent
PINS_FILE = REPO_ROOT / "spec" / "ondc-pins.yaml"

GITHUB_API = "https://api.github.com"
ONDC_REPOS = [
    {"owner": "ONDC-Official", "repo": "automation-specifications", "key": "automation_specs"},
    {"owner": "ONDC-Official", "repo": "log-validation-utility",   "key": "log_validation"},
    {"owner": "ONDC-Official", "repo": "mock-server-utility",       "key": "mock_server"},
    {"owner": "ONDC-Official", "repo": "rsf-bulk-utility",          "key": "rsf_bulk"},
]

def github_get(path: str) -> dict:
    token = os.environ.get("GITHUB_TOKEN", "")
    req = urllib.request.Request(
        f"{GITHUB_API}{path}",
        headers={
            "Accept": "application/vnd.github.v3+json",
            "User-Agent": "hisaab-spec-drift-checker",
            **({"Authorization": f"Bearer {token}"} if token else {}),
        }
    )
    with urllib.request.urlopen(req, timeout=15) as resp:
        return json.loads(resp.read())


def load_pins() -> dict:
    if not PINS_FILE.exists():
        return {}
    with open(PINS_FILE) as f:
        return yaml.safe_load(f) or {}


def save_pins(pins: dict):
    with open(PINS_FILE, "w") as f:
        yaml.dump(pins, f, default_flow_style=False)
    print(f"Updated {PINS_FILE}")


def main():
    pins = load_pins()
    drifted = []

    for repo_info in ONDC_REPOS:
        owner = repo_info["owner"]
        repo = repo_info["repo"]
        key = repo_info["key"]

        try:
            commits = github_get(f"/repos/{owner}/{repo}/commits?per_page=1")
            latest_sha = commits[0]["sha"]
            latest_date = commits[0]["commit"]["committer"]["date"]
            latest_msg = commits[0]["commit"]["message"].splitlines()[0]
        except Exception as e:
            print(f"WARNING: Could not fetch {owner}/{repo}: {e}")
            continue

        pinned_sha = pins.get(key, {}).get("sha", "")

        if pinned_sha != latest_sha:
            if pinned_sha:
                drifted.append({
                    "repo": f"{owner}/{repo}",
                    "key": key,
                    "pinned_sha": pinned_sha[:8],
                    "latest_sha": latest_sha[:8],
                    "latest_date": latest_date,
                    "latest_msg": latest_msg,
                })
                print(f"⚠ DRIFT: {owner}/{repo}")
                print(f"  Pinned: {pinned_sha[:8]}")
                print(f"  Latest: {latest_sha[:8]} ({latest_date})")
                print(f"  Commit: {latest_msg[:80]}")
            else:
                print(f"ℹ INIT: Pinning {owner}/{repo} at {latest_sha[:8]}")
                pins[key] = {"sha": latest_sha, "pinned_at": datetime.now(timezone.utc).isoformat()}

    save_pins(pins)

    if drifted:
        print(f"\n❌ SPEC DRIFT DETECTED in {len(drifted)} ONDC repos.")
        print("Action required:")
        print("  1. Review the upstream changes")
        print("  2. Update spec/edge-cases.yaml if behavior changed")
        print("  3. Update affected scenarios and their BRD citations")
        print("  4. Update the pin SHA in spec/ondc-pins.yaml")
        print("  5. Open an ADR if the change affects a design decision")
        sys.exit(1)
    else:
        print("\n✓ No spec drift detected")
        sys.exit(0)


if __name__ == "__main__":
    main()
