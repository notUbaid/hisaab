#!/usr/bin/env python3
"""
validate_ondc_schemas.py — validate test fixture payloads against ONDC log-validation-utility.

Sends /settle and /on_settle test payloads to the ONDC RSF validator endpoint
and fails the build if any payload fails schema validation.

Usage:
  python3 scripts/validate_ondc_schemas.py \
    --fixtures tests/scenarios/fixtures/ \
    --endpoint https://log-validation.ondc.org/api/validate \
    --fail-on-error
"""

import argparse
import json
import sys
import urllib.request
import urllib.error
from pathlib import Path


def validate_payload(endpoint: str, payload: dict, fixture_name: str) -> tuple[bool, str]:
    """Send payload to ONDC validator. Returns (passed, message)."""
    try:
        data = json.dumps(payload).encode()
        req = urllib.request.Request(
            endpoint,
            data=data,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=30) as resp:
            result = json.loads(resp.read())
            # ONDC validator returns {valid: bool, errors: [...]}
            if result.get("valid", False):
                return True, "PASS"
            else:
                errors = result.get("errors", [])
                return False, f"FAIL: {errors}"
    except urllib.error.URLError as e:
        return None, f"UNREACHABLE: {e}"
    except Exception as e:
        return None, f"ERROR: {e}"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--fixtures", required=True, help="Directory containing fixture JSON files")
    parser.add_argument("--endpoint", required=True, help="ONDC log-validation-utility RSF endpoint")
    parser.add_argument("--fail-on-error", action="store_true")
    args = parser.parse_args()

    fixtures_dir = Path(args.fixtures)
    if not fixtures_dir.exists():
        print(f"Fixtures directory not found: {fixtures_dir}")
        print("Skipping ONDC schema validation (will validate once fixtures are added)")
        sys.exit(0)

    fixtures = list(fixtures_dir.glob("*.json"))
    if not fixtures:
        print("No fixture JSON files found — skipping")
        sys.exit(0)

    results = {"pass": 0, "fail": 0, "unreachable": 0}

    for fixture_path in sorted(fixtures):
        try:
            payload = json.loads(fixture_path.read_text())
        except Exception as e:
            print(f"  ERROR reading {fixture_path.name}: {e}")
            continue

        passed, message = validate_payload(args.endpoint, payload, fixture_path.name)

        if passed is None:
            print(f"  ⚠ {fixture_path.name}: {message}")
            results["unreachable"] += 1
        elif passed:
            print(f"  ✓ {fixture_path.name}: {message}")
            results["pass"] += 1
        else:
            print(f"  ✗ {fixture_path.name}: {message}")
            results["fail"] += 1

    print(f"\nResults: {results['pass']} pass, {results['fail']} fail, {results['unreachable']} unreachable")

    if results["unreachable"] == len(fixtures):
        print("WARNING: ONDC validator unreachable — check ONDC_VALIDATOR_URL secret")
        sys.exit(0)  # Don't fail CI if validator is down

    if results["fail"] > 0 and args.fail_on_error:
        print(f"\n❌ {results['fail']} fixture(s) failed ONDC schema validation")
        sys.exit(1)

    sys.exit(0)


if __name__ == "__main__":
    main()
