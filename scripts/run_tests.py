#!/usr/bin/env python3
"""
scripts/run_tests.py — Runs Go test packages by compiling to bin/ to comply with Windows AppLocker policy.
"""
import os
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).parent.parent
BIN_DIR = REPO_ROOT / "bin"
BIN_DIR.mkdir(exist_ok=True)

PACKAGES = [
    "cmd/api",
    "cmd/worker",
    "internal/audit",
    "internal/auth",
    "internal/calendar",
    "internal/ledger",
    "internal/nptoolkit",
    "internal/recon",
    "internal/scenariodsl",
    "internal/settlement",
    "internal/verification",
    "tests/adversarial",
    "tests/crash",
    "tests/mutation",
    "tests/property",
    "tests/scenarios",
]

def run():
    failed = []
    passed = []

    for pkg in PACKAGES:
        pkg_name = pkg.replace("/", "_")
        exe_path = BIN_DIR / f"{pkg_name}_suite.exe"
        pkg_path = f"./{pkg}"

        print(f"=== Compiling & Running {pkg} ===")
        # Compile test binary
        build_cmd = ["go", "test", "-c", "-o", str(exe_path), pkg_path]
        res = subprocess.run(build_cmd, cwd=REPO_ROOT, capture_output=True, text=True)
        if res.returncode != 0:
            print(f"FAIL [compile] {pkg}:\n{res.stderr}")
            failed.append(pkg)
            continue

        # Execute test binary
        run_cmd = [str(exe_path), "-test.v=true"]
        res = subprocess.run(run_cmd, cwd=REPO_ROOT, capture_output=True, text=True)
        if res.returncode != 0:
            print(f"FAIL [run] {pkg}:\n{res.stdout}\n{res.stderr}")
            failed.append(pkg)
        else:
            print(f"PASS {pkg}")
            passed.append(pkg)

    print("\n" + "="*50)
    print(f"Results: {len(passed)} PASSED, {len(failed)} FAILED")
    print("="*50)
    if failed:
        print("Failed packages:", failed)
        sys.exit(1)
    else:
        print("All test packages passed cleanly!")

if __name__ == "__main__":
    run()
