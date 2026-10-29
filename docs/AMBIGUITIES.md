# ONDC RSF 2.0 Spec Ambiguity Register

> **Principle:** When the specification is ambiguous, Hisaab never silently guesses.
> Every ambiguity is documented here, registered in `spec/ambiguities.yaml`, and controlled via explicit policy flags.

---

### AMB-001: Match Semantics (Order ID vs Components)
- **Reference:** BRD §4.2 & Annexure Worked Example
- **Issue:** Seller claims orders 1–4, Collector claims 2–5. Orders 3 and 4 are flagged matched despite differing Buyer Finder Fee amounts.
- **Policy Flag:** `match_amount_tolerance_paise`
- **Default Policy:** `strict_all_components` (all amounts must balance to 0 paise tolerance).

### AMB-002: Insufficient Balance Behavior
- **Reference:** BRD §4.2 Case 12
- **Issue:** When Collector settlement balance is insufficient, does the SA reject all orders or support partial FIFO fills?
- **Policy Flag:** `partial_fill_allowed`
- **Default Policy:** `all_or_nothing` (standard BRD behavior).

### AMB-003: Carry-Forward Retention Limit
- **Reference:** BRD §4.2 Cases 9, 10, 11
- **Issue:** No explicit horizon given for how many days unmatched claims remain active.
- **Policy Flag:** `carry_forward_days`
- **Default Policy:** `2_business_days` (aligns with T+2 ONDC delivery-to-settlement guidance).

### AMB-004: Cut-Off Window Time & Timezone Definition
- **Reference:** BRD §3.1 Settlement Cycle
- **Issue:** Spec says "same date" without specifying cut-off time (e.g. 18:00 vs 23:59:59 IST) or time authority.
- **Policy Flag:** `cut_off_time_ist`
- **Default Policy:** `23_59_59_server_ist` (monotonic server reception time in Asia/Kolkata).

### AMB-005: Tax Withholding (GST TCS / TDS) Allocation
- **Reference:** BRD Annexure §5
- **Issue:** BRD claim schemas omit dedicated tax withholding fields; unclear whether SA withholds or NPs pre-withhold.
- **Policy Flag:** `tax_reconciliation_mode`
- **Default Policy:** `explicit_deduction_legs` (SA calculates statutory deductions as discrete ledger legs).

### AMB-006: Arithmetic Discrepancy in BRD Worked Example
- **Reference:** BRD Annexure Page 14
- **Issue:** Example states matched total of ₹200, but orders 2, 3, and 4 sum to ₹300 (₹100 each). Text states order 3 is both processed and rejected.
- **Policy Flag:** `brd_example_arithmetic_oracle`
- **Default Policy:** `strict_arithmetic_correction` (true sum ₹300 used in tests).

### AMB-007: Seller on Record (SoR) Payout Failure Treatment
- **Reference:** BRD §4.2 Case 21
- **Issue:** When SoR bank account is invalid, does SA reject entire order or settle Seller App commission and route SoR leg to suspense?
- **Policy Flag:** `sor_failure_mode`
- **Default Policy:** `split_suspense_routing` (isolate failure to defective beneficiary leg).

### AMB-008: Bank Instruction Failure Recovery
- **Reference:** BRD §4.2 Case 14
- **Issue:** Reversal posting vs manual suspense clearing when bank instruction fails after account debit.
- **Policy Flag:** `bank_failure_recovery`
- **Default Policy:** `auto_reversing_journal` (automated reversing entry maintains ledger consistency).
