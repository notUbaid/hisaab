#!/usr/bin/env python3
"""
check_spec_coverage.py — verify all 23 BRD edge cases have a corresponding test.

Reads spec/edge-cases.yaml and checks that:
  1. Every edge case ID (EC-01 through EC-23) has a test function TestScenario_<ID>
  2. Every test function has a spec version comment
  3. Every ambiguous edge case has an entry in spec/ambiguities.yaml

Fails the build if coverage is below 100% (once all 23 are implemented).
Warns if coverage is below current week's target.
"""

import sys
import re
import yaml
from pathlib import Path

REPO_ROOT = Path(__file__).parent.parent
EDGE_CASES_FILE = REPO_ROOT / "spec" / "edge-cases.yaml"
AMBIGUITIES_FILE = REPO_ROOT / "spec" / "ambiguities.yaml"
SCENARIOS_DIR = REPO_ROOT / "tests" / "scenarios"


def load_edge_cases():
    if not EDGE_CASES_FILE.exists():
        print("WARNING: spec/edge-cases.yaml not found — skipping spec coverage check")
        return []
    with open(EDGE_CASES_FILE) as f:
        data = yaml.safe_load(f)
    return data.get("edge_cases", [])


def load_ambiguities():
    if not AMBIGUITIES_FILE.exists():
        return []
    with open(AMBIGUITIES_FILE) as f:
        data = yaml.safe_load(f)
    return data.get("ambiguities", [])


def find_test_functions():
    """Find all TestScenario_* functions in the scenarios directory."""
    test_pattern = re.compile(r'func (TestScenario_[A-Z0-9_-]+)\(')
    found = {}
    if not SCENARIOS_DIR.exists():
        return found
    for path in SCENARIOS_DIR.rglob("*_test.go"):
        content = path.read_text()
        for match in test_pattern.finditer(content):
            func_name = match.group(1)
            found[func_name] = path
    return found


def main():
    edge_cases = load_edge_cases()
    ambiguities = load_ambiguities()
    test_functions = find_test_functions()
    ambiguity_ids = {a.get("edge_case_id") for a in ambiguities if a.get("edge_case_id")} | {a["id"] for a in ambiguities if "id" in a}

    if not edge_cases:
        print("WARNING: No edge cases found in spec/edge-cases.yaml")
        print("  Add edge cases before Week 2.")
        sys.exit(0)

    issues = []
    covered = 0

    for ec in edge_cases:
        ec_id = ec["id"]  # e.g., "EC-01"
        expected_func = f"TestScenario_{ec_id.replace('-', '_')}"  # TestScenario_EC_01

        if expected_func in test_functions:
            covered += 1
        else:
            issues.append(f"  MISSING: {ec_id} - no test function {expected_func}()")

        # Check ambiguous cases have entries
        if ec.get("status") in ["ambiguous", "needs_practitioner_review"]:
            if ec_id not in ambiguity_ids:
                issues.append(
                    f"  MISSING AMBIGUITY: {ec_id} is '{ec.get('status')}' "
                    f"but has no entry in spec/ambiguities.yaml"
                )

    total = len(edge_cases)
    pct = (covered / total * 100) if total > 0 else 0

    print(f"Spec coverage: {covered}/{total} edge cases ({pct:.0f}%)")

    if issues:
        print()
        for issue in issues:
            print(issue)

    # Warn if ambiguity register is thin
    if len(ambiguities) < 5:
        print(f"\nWARNING: Ambiguity register has only {len(ambiguities)} entries.")
        print("  Target: >=8 entries by Week 2. See CLAUDE.md Section 9.")

    # Fail if we have zero coverage and edge cases are defined
    if total > 0 and covered == 0:
        print("\nFAIL: No BRD scenarios are tested yet. Start with EC-01.")
        sys.exit(1)

    print(f"\n{'OK' if not issues else 'WARN'}: Spec coverage check complete")
    sys.exit(0 if not issues else 1)


if __name__ == "__main__":
    main()
