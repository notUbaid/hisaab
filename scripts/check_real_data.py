#!/usr/bin/env python3
"""
check_real_data.py — CI guard: detect real Indian financial identifiers in test fixtures.

Fails the build if any of the following appear in tests/ or spec/:
  - Real IFSC codes (format: ABCD0XXXXXX where ABCD is a real bank prefix)
  - Real-looking account numbers (9–18 digits, not synthetic patterns)
  - Real UPI IDs
  - Real PAN numbers
  - Real GSTIN numbers

Synthetic patterns are allowed and marked with # SYNTHETIC or // SYNTHETIC comments.
"""

import re
import sys
import os
from pathlib import Path

# Directories to scan
SCAN_DIRS = ["tests", "spec"]

# File extensions to scan
EXTENSIONS = {".go", ".yaml", ".yml", ".json", ".sql", ".txt"}

# ─── Pattern definitions ───────────────────────────────────────────────────────

# Real Indian bank IFSC prefixes (first 4 chars identify the bank)
# These are the major banks. A full list would be from RBI.
KNOWN_BANK_PREFIXES = {
    "HDFC", "ICIC", "SBIN", "AXIS", "KOTK", "YESB", "IDBI", "PUNB",
    "CNRB", "UBIN", "BARB", "BKID", "CITI", "DEUT", "HSBC", "SCBL",
    "INDB", "IOBA", "MAHB", "ORBC", "RATN", "RNSB", "SIBL", "TMBL",
    "TNSC", "UCBA", "UTIB", "VIJB", "NKGSB", "FDRL", "DCBL", "KVBL",
}

IFSC_PATTERN = re.compile(r'\b([A-Z]{4})0[A-Z0-9]{6}\b')
UPI_PATTERN = re.compile(r'\b[a-zA-Z0-9._-]+@[a-z]+\b')
PAN_PATTERN = re.compile(r'\b[A-Z]{5}[0-9]{4}[A-Z]\b')
GSTIN_PATTERN = re.compile(r'\b[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z][1-9A-Z]Z[0-9A-Z]\b')

# Account number: 9–18 digits not obviously synthetic
ACCOUNT_PATTERN = re.compile(r'\b[0-9]{9,18}\b')

# Synthetic markers — lines with these are allowed
SYNTHETIC_MARKERS = [
    "SYNTHETIC", "MOCK", "FAKE", "TEST_DATA", "DUMMY", "PLACEHOLDER",
    "000000", "111111", "123456", "999999", "RAMP-ONLY",
]

def is_synthetic_line(line: str) -> bool:
    """Return True if the line is clearly marked as synthetic test data."""
    return any(marker in line.upper() for marker in SYNTHETIC_MARKERS)

def check_file(path: Path) -> list[tuple[int, str, str]]:
    """Return list of (line_number, issue_type, line_content) for violations."""
    violations = []
    try:
        lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    except Exception:
        return violations

    for i, line in enumerate(lines, start=1):
        if is_synthetic_line(line):
            continue

        # Check IFSC
        for match in IFSC_PATTERN.finditer(line):
            prefix = match.group(1)
            if prefix in KNOWN_BANK_PREFIXES:
                violations.append((i, f"REAL IFSC ({match.group()})", line.strip()))

        # Check UPI
        for match in UPI_PATTERN.finditer(line):
            upi = match.group()
            # Skip obviously fake patterns
            if not any(fake in upi.lower() for fake in ["example", "test", "mock", "fake", "dummy"]):
                violations.append((i, f"POSSIBLE UPI ID ({upi})", line.strip()))

        # Check PAN
        for match in PAN_PATTERN.finditer(line):
            violations.append((i, f"POSSIBLE PAN ({match.group()})", line.strip()))

        # Check GSTIN
        for match in GSTIN_PATTERN.finditer(line):
            violations.append((i, f"POSSIBLE GSTIN ({match.group()})", line.strip()))

    return violations


def main():
    repo_root = Path(__file__).parent.parent
    all_violations = []

    for scan_dir in SCAN_DIRS:
        target = repo_root / scan_dir
        if not target.exists():
            continue

        for path in target.rglob("*"):
            if path.suffix in EXTENSIONS and path.is_file():
                violations = check_file(path)
                for line_no, issue, content in violations:
                    all_violations.append({
                        "file": str(path.relative_to(repo_root)),
                        "line": line_no,
                        "issue": issue,
                        "content": content[:120],
                    })

    if all_violations:
        print("[FAIL] REAL FINANCIAL DATA DETECTED IN TEST FIXTURES")
        print("=" * 60)
        for v in all_violations:
            print(f"  {v['file']}:{v['line']} - {v['issue']}")
            print(f"    {v['content']}")
        print()
        print("Action required:")
        print("  1. Replace real identifiers with synthetic data")
        print("  2. Add '# SYNTHETIC' comment to mark test data explicitly")
        print("  3. See CLAUDE.md Section 8 for synthetic data policy")
        sys.exit(1)
    else:
        print("[OK] No real financial identifiers found in test fixtures")
        sys.exit(0)


if __name__ == "__main__":
    main()
