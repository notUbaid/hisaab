# Physical Browser Verification Report

> Automated browser verification executed via Playwright with headless Google Chrome (`C:\Program Files\Google\Chrome\Application\chrome.exe`).  
> All 4 core operational routes of the Next.js 15 Ops Console were inspected for visual rendering, layout integrity, and real-time interaction.

---

## 1. Test Execution Summary

- **Test Runner:** `scripts/physical_browser_check.py`
- **Browser:** Google Chrome (Headless, Viewport 1440x900)
- **Target URL:** `http://localhost:3000`
- **Execution Date:** 2026-10-30 01:20:35 IST (`Asia/Kolkata`)
- **Console Errors:** 0
- **Overall Status:** **100% PASS**

---

## 2. Tested Routes & Verified Elements

### Route 1: Operations Dashboard (`/`)
- **Page Header:** `Operations & Correctness Console`
- **Metric Cards:**
  - Active Invariants: `15 / 15` (Formally Enforced & Attacked)
  - BRD Spec Coverage: `23 / 23` (100% Executable Scenarios)
  - Settlement Volume: `₹1,54,200.00`
  - Reconciliation Balance: `0.00 paise` (Zero trial balance delta)
- **Batches Table:** Rendered with status badges (`SETTLED`, `MATCHING`, `EXCEPTION`).
- **Core Invariants Sidebar:** Summaries of Invariants 1, 3, 4, and 13.
- **Screenshot Artifact:** `.playwright_artifacts/01_dashboard.png`

### Route 2: Settlement Batch Timeline (`/timeline`)
- **Page Header:** `Settlement Batch Timeline`
- **Batch Banner:** `BATCH-20261030-01` (`bap.dunzo.com` &harr; `bpp.blinkit.com`)
  - Gross Volume: `₹4,200.00`
  - Buyer Finder Fee (BFF): `₹210.00`
  - Net Instructed: `₹3,990.00`
  - Terminal Status: `SETTLED (Verified)`
- **5-Step Traceable Lifecycle:**
  1. *Step 1: Independent Claim Submissions* (18:00:15 IST) — Ed25519 signatures verified.
  2. *Step 2: Order-Level Matching* (18:00:18 IST) — 42/42 orders matched 100%.
  3. *Step 3: Collector Balance Verification (EC-12)* (18:00:20 IST) — Balance check PASSED.
  4. *Step 4: Net Bank Payment Instruction Dispatch* (18:00:25 IST) — Invariant 13 ties out.
  5. *Step 5: Bank Credit Confirmation & Sub-Ledger Posting* (18:01:05 IST) — Merkle root sealed.
- **Screenshot Artifact:** `.playwright_artifacts/02_timeline.png`

### Route 3: Reconciliation Exception Queue (`/exceptions`)
- **Page Header:** `Reconciliation Exception Queue`
- **Taxonomy Breakdown:** 11-class operational taxonomy with business-day ageing.
- **Open Breaks Summary:** 3 Open Breaks, 1 Pending Maker-Checker.
- **Verified Rows:**
  - `EXC-20261030-01`: `INSUFFICIENT_BALANCE` (ORD-99120, Ageing: 2 business days).
  - `EXC-20261030-02`: `AMOUNT_MISMATCH` (ORD-88419, Ageing: 1 business day).
  - `EXC-20261030-03`: `BANK_FAILURE` (ORD-77102, Ageing: 3 business days, `PENDING_APPROVAL`).
  - `EXC-20261030-04`: `UNCLAIMED_BANK_CREDIT` (UTR-551029, Ageing: 0 business days).
- **Screenshot Artifact:** `.playwright_artifacts/03_exceptions.png`

### Route 4: Scenario Playground (`/playground`)
- **Page Header:** `Scenario Playground`
- **Edge Case Directory:** All **23 BRD edge cases (`EC-01` to `EC-23`)** navigable on the left panel with BRD section citations and policy flag badges.
- **Interactive Verification 1 (`EC-03` - Identical Order Sets Matched 100%):**
  - Clicked and executed in browser.
  - Verification trace verified: Protocol validation (Digest, Ed25519), RSF 2.0 Engine Outcome (`PROCESSED_CLEAN`), and 6 formal invariants verified (`100% Invariant Compliant`).
  - Screenshot Artifact: `.playwright_artifacts/04_playground_ec03.png`
- **Interactive Verification 2 (`EC-08` - Amount Mismatch):**
  - Clicked and executed in browser.
  - Verified error badge `AMOUNT_MISMATCH` and active policy flag `match_amount_tolerance_paise: 0`.
  - Screenshot Artifact: `.playwright_artifacts/05_playground_ec08.png`

---

## 3. How to Reproduce

```bash
# 1. Start console server in one terminal
cd console
npm run start -- -p 3000

# 2. In another terminal, run the automated browser test
python scripts/physical_browser_check.py
```
