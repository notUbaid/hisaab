# ONDC RSF 2.0 Spec Coverage Matrix

**Target:** 23 of 23 BRD Edge Cases Executable & Verified  
**Pinned Spec:** NTS10 2.0.0 / RSF 2.0 BRD (dated 2 Aug 2024)

| Edge Case ID | Title | BRD Section | Scenario Test | Status |
|---|---|---|---|---|
| EC-01 | Buyer App submits /settle; Seller App does not submit | §4.2 Case 1 | `TestScenario_EC_01` | Scaffolded |
| EC-02 | Seller App submits /settle; Buyer App does not submit | §4.2 Case 2 | `TestScenario_EC_02` | Scaffolded |
| EC-03 | Both Buyer App and Seller App submit identical order sets | §4.2 Case 3 | `TestScenario_EC_03` | Scaffolded |
| EC-04 | Partial order set overlap between Buyer and Seller claims | §4.2 Case 4 | `TestScenario_EC_04` | Scaffolded |
| EC-05 | Claim contains mixed counterparty NP pairs in single /settle | §4.2 Case 5 | `TestScenario_EC_05` | Scaffolded |
| EC-06 | Logistics fees bundled in core order /settle payload | §4.2 Case 6 | `TestScenario_EC_06` | Scaffolded |
| EC-07 | Claim missing mandatory schema fields | §4.2 Case 7 | `TestScenario_EC_07` | Scaffolded |
| EC-08 | Amount mismatch on matched Network Order ID | §4.2 Case 8 | `TestScenario_EC_08` | Scaffolded |
| EC-09 | Late counterpart arrival within carry-forward window | §4.2 Case 9 | `TestScenario_EC_09` | Scaffolded |
| EC-10 | Counterpart arrives after carry-forward expiration | §4.2 Case 10 | `TestScenario_EC_10` | Scaffolded |
| EC-11 | Duplicate /settle submission with identical message_id | §4.2 Case 11 | `TestScenario_EC_11` | Scaffolded |
| EC-12 | Insufficient balance in Collector settlement account | §4.2 Case 12 | `TestScenario_EC_12` | Scaffolded |
| EC-13 | Bank timeout on instruction submission | §4.2 Case 13 | `TestScenario_EC_13` | Scaffolded |
| EC-14 | Bank returns explicit failure (Account frozen / Invalid IFSC) | §4.2 Case 14 | `TestScenario_EC_14` | Scaffolded |
| EC-15 | Duplicate credit notification from bank | §4.2 Case 15 | `TestScenario_EC_15` | Scaffolded |
| EC-16 | Refund claim with funds still in Collector settlement account | §4.2 Case 16 | `TestScenario_EC_16` | Scaffolded |
| EC-17 | Refund claim after forward payout already credited to Seller | §4.2 Case 17 | `TestScenario_EC_17` | Scaffolded |
| EC-18 | Refund claim with insufficient Seller balance | §4.2 Case 18 | `TestScenario_EC_18` | Scaffolded |
| EC-19 | Claim submitted on Indian Bank Holiday or Weekend | §4.2 Case 19 | `TestScenario_EC_19` | Scaffolded |
| EC-20 | Cut-off timestamp boundary race (IST 23:59:59) | §4.2 Case 20 | `TestScenario_EC_20` | Scaffolded |
| EC-21 | Seller on Record (SoR) account validation failure | §4.2 Case 21 | `TestScenario_EC_21` | Scaffolded |
| EC-22 | Deduction leg mismatch (GST TCS / TDS calculated differently) | §4.2 Case 22 | `TestScenario_EC_22` | Scaffolded |
| EC-23 | System crash mid-batch between gross order legs and net batch posting | §4.2 Case 23 | `TestScenario_EC_23` | Scaffolded |
