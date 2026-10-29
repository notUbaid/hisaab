"use client";

import React, { useState } from "react";
import { formatPaiseToINR } from "@/lib/rbac";

interface BatchData {
  id: string;
  npPair: string;
  grossVolume: number;
  bff: number;
  netInstructed: number;
  status: "SETTLED" | "MATCHING" | "EXCEPTION";
  merkleRoot: string;
  steps: {
    step: number;
    name: string;
    time: string;
    status: "COMPLETED" | "IN_PROGRESS" | "FAILED" | "PENDING";
    description: string;
    details: string;
  }[];
}

const BATCHES: BatchData[] = [
  {
    id: "BATCH-20261030-01",
    npPair: "bap.dunzo.com ↔ bpp.blinkit.com",
    grossVolume: 420000,
    bff: 21000,
    netInstructed: 399000,
    status: "SETTLED",
    merkleRoot: "sha256:7f83b1657ff1fc53b92dc18148a1d65dfc2d4b1fa3d677284addd200126d9069",
    steps: [
      {
        step: 1,
        name: "Independent Claim Submissions",
        time: "18:00:15 IST",
        status: "COMPLETED",
        description: "Buyer App (bap.dunzo.com) and Seller App (bpp.blinkit.com) submit /settle payloads for NP pair.",
        details: "Buyer submitted 42 orders; Seller submitted 42 orders. Both signatures verified via Ed25519.",
      },
      {
        step: 2,
        name: "Order-Level Matching",
        time: "18:00:18 IST",
        status: "COMPLETED",
        description: "SA simulator reconciles claims order-by-order on Network Order ID and amount components.",
        details: "42 of 42 orders matched 100%. Total Matched Volume: ₹4,200.00. Total BFF: ₹210.00.",
      },
      {
        step: 3,
        name: "Collector Balance Verification (EC-12)",
        time: "18:00:20 IST",
        status: "COMPLETED",
        description: "SA checks Collector ondc_settlement balance against required net transfer.",
        details: "Required Transfer: ₹3,990.00 (Total Matched - BFF). Available Balance: ₹15,000.00. PASSED.",
      },
      {
        step: 4,
        name: "Net Bank Payment Instruction Dispatch",
        time: "18:00:25 IST",
        status: "COMPLETED",
        description: "Gross obligations netted; net instruction generated for bank settlement rail.",
        details: "Debit collector:ondc_settlement (₹3,990.00) -> Credit receiver:ondc_settlement (₹3,990.00). Invariant 13 ties out.",
      },
      {
        step: 5,
        name: "Bank Credit Confirmation & Sub-Ledger Posting",
        time: "18:01:05 IST",
        status: "COMPLETED",
        description: "Bank returns UTR confirmation; atomic double-entry subledger journals committed.",
        details: "UTR: HDFC20261030009941. Immutability trigger verified. Batch Merkle root sealed offline.",
      },
    ],
  },
  {
    id: "BATCH-20261030-02",
    npPair: "bap.paytm.com ↔ bpp.zepto.com",
    grossVolume: 1280000,
    bff: 64000,
    netInstructed: 1216000,
    status: "SETTLED",
    merkleRoot: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    steps: [
      {
        step: 1,
        name: "Independent Claim Submissions",
        time: "18:15:10 IST",
        status: "COMPLETED",
        description: "Paytm and Zepto submit claims for 128 matched grocery orders.",
        details: "128 claim lines validated against Beckn RSF 2.0 schema without warnings.",
      },
      {
        step: 2,
        name: "Order-Level Matching",
        time: "18:15:14 IST",
        status: "COMPLETED",
        description: "Exact match across all 128 orders.",
        details: "Volume: ₹12,800.00. Zero amount mismatches. Invariant 5 (Conservation) verified.",
      },
      {
        step: 3,
        name: "Collector Balance Verification (EC-12)",
        time: "18:15:17 IST",
        status: "COMPLETED",
        description: "Balance check executed.",
        details: "Available: ₹250,000.00. Required: ₹12,160.00. PASSED.",
      },
      {
        step: 4,
        name: "Net Bank Payment Instruction Dispatch",
        time: "18:15:22 IST",
        status: "COMPLETED",
        description: "Dispatched to ICICI Corporate Banking API.",
        details: "Net transfer of ₹12,160.00 instructed successfully.",
      },
      {
        step: 5,
        name: "Bank Credit Confirmation & Sub-Ledger Posting",
        time: "18:16:00 IST",
        status: "COMPLETED",
        description: "Settlement finalized.",
        details: "UTR: ICIC20261030018442. Merkle root verified and anchored.",
      },
    ],
  },
  {
    id: "BATCH-20261030-03",
    npPair: "bap.phonepe.com ↔ bpp.swiggy.com",
    grossVolume: 180000,
    bff: 9000,
    netInstructed: 171000,
    status: "MATCHING",
    merkleRoot: "sha256:pending_batch_computation",
    steps: [
      {
        step: 1,
        name: "Independent Claim Submissions",
        time: "18:30:02 IST",
        status: "COMPLETED",
        description: "PhonePe submitted 18 orders; Swiggy submitted 18 orders.",
        details: "Both inbound payloads authenticated.",
      },
      {
        step: 2,
        name: "Order-Level Matching",
        time: "18:30:05 IST",
        status: "IN_PROGRESS",
        description: "Reconciling order components and tax withholding legs.",
        details: "16 of 18 orders matched. 2 orders undergoing component breakdown analysis.",
      },
      {
        step: 3,
        name: "Collector Balance Verification (EC-12)",
        time: "--:--:--",
        status: "PENDING",
        description: "Awaiting matching completion.",
        details: "Estimated transfer: ₹1,710.00.",
      },
      {
        step: 4,
        name: "Net Bank Payment Instruction Dispatch",
        time: "--:--:--",
        status: "PENDING",
        description: "Pending balance clearance.",
        details: "Instruction not yet formed.",
      },
      {
        step: 5,
        name: "Bank Credit Confirmation & Sub-Ledger Posting",
        time: "--:--:--",
        status: "PENDING",
        description: "Pending bank dispatch.",
        details: "Ledger entries unposted.",
      },
    ],
  },
  {
    id: "BATCH-20261030-04",
    npPair: "bap.tata1mg.com ↔ bpp.apollo.com",
    grossVolume: 30000,
    bff: 1500,
    netInstructed: 0,
    status: "EXCEPTION",
    merkleRoot: "sha256:quarantined_due_to_balance_failure",
    steps: [
      {
        step: 1,
        name: "Independent Claim Submissions",
        time: "18:45:00 IST",
        status: "COMPLETED",
        description: "Tata 1mg and Apollo submit 3 pharmacy orders.",
        details: "Payloads validated.",
      },
      {
        step: 2,
        name: "Order-Level Matching",
        time: "18:45:03 IST",
        status: "COMPLETED",
        description: "All 3 orders matched on gross amount ₹300.00.",
        details: "BFF: ₹15.00. Net Due: ₹285.00.",
      },
      {
        step: 3,
        name: "Collector Balance Verification (EC-12)",
        time: "18:45:05 IST",
        status: "FAILED",
        description: "Collector ondc_settlement balance insufficient.",
        details: "Available: ₹150.00. Required: ₹285.00. Deficit: ₹135.00. Standard All-or-Nothing policy triggered.",
      },
      {
        step: 4,
        name: "Net Bank Payment Instruction Dispatch",
        time: "18:45:06 IST",
        status: "FAILED",
        description: "Instruction aborted.",
        details: "Zero bank instructions generated. Quarantined to Recon Exception Queue.",
      },
      {
        step: 5,
        name: "Bank Credit Confirmation & Sub-Ledger Posting",
        time: "18:45:06 IST",
        status: "FAILED",
        description: "Ledger mutation prevented.",
        details: "Invariant 4 (Non-Negative Balance) protected. Exception EXC-20261030-01 logged.",
      },
    ],
  },
];

export default function TimelinePage() {
  const [selectedBatchId, setSelectedBatchId] = useState<string>("BATCH-20261030-01");
  const currentBatch = BATCHES.find((b) => b.id === selectedBatchId) || BATCHES[0];

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-white">Settlement Batch Timeline</h1>
          <p className="text-sm text-zinc-400 mt-1">
            Traceable lifecycle of batch <code className="text-zinc-200 font-mono">{currentBatch.id}</code> ({currentBatch.npPair})
          </p>
        </div>

        {/* Batch Selector Tabs */}
        <div className="flex flex-wrap gap-1 p-1 rounded-lg bg-zinc-900 border border-zinc-800 text-xs font-mono">
          {BATCHES.map((b) => (
            <button
              key={b.id}
              onClick={() => setSelectedBatchId(b.id)}
              className={`px-3 py-1.5 rounded-md transition cursor-pointer ${
                selectedBatchId === b.id
                  ? "bg-zinc-800 text-white font-bold shadow-sm"
                  : "text-zinc-400 hover:text-zinc-200"
              }`}
            >
              {b.id} ({b.status})
            </button>
          ))}
        </div>
      </div>

      {/* Batch Overview Banner */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 p-4 rounded-xl border border-zinc-800 bg-zinc-900/40 font-mono text-xs">
        <div>
          <span className="text-zinc-500 uppercase">Gross Volume:</span>
          <div className="text-zinc-200 font-bold text-sm mt-0.5">{formatPaiseToINR(currentBatch.grossVolume)}</div>
        </div>
        <div>
          <span className="text-zinc-500 uppercase">Buyer Finder Fee (BFF):</span>
          <div className="text-zinc-200 font-bold text-sm mt-0.5">{formatPaiseToINR(currentBatch.bff)}</div>
        </div>
        <div>
          <span className="text-zinc-500 uppercase">Net Instructed:</span>
          <div className="text-emerald-400 font-bold text-sm mt-0.5">{formatPaiseToINR(currentBatch.netInstructed)}</div>
        </div>
        <div>
          <span className="text-zinc-500 uppercase">Terminal Status:</span>
          <div className="mt-0.5">
            <span
              className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                currentBatch.status === "SETTLED"
                  ? "bg-emerald-950 text-emerald-400 border border-emerald-800"
                  : currentBatch.status === "MATCHING"
                  ? "bg-amber-950 text-amber-400 border border-amber-800"
                  : "bg-rose-950 text-rose-400 border border-rose-800"
              }`}
            >
              {currentBatch.status}
            </span>
          </div>
        </div>
      </div>

      {/* Merkle Root & Tie-Out Box */}
      <div className="p-3.5 rounded-lg border border-zinc-800/80 bg-zinc-950 font-mono text-xs flex flex-col sm:flex-row sm:items-center justify-between gap-2">
        <div className="truncate">
          <span className="text-zinc-500 text-[10px] uppercase">Merkle Root: </span>
          <span className="text-zinc-300 text-[11px]">{currentBatch.merkleRoot}</span>
        </div>
        <div className="text-emerald-400 font-medium text-[11px] whitespace-nowrap">
          Invariant 13 Tie-Out: Gross ({formatPaiseToINR(currentBatch.grossVolume)}) &minus; BFF ({formatPaiseToINR(currentBatch.bff)}) = Net ({formatPaiseToINR(currentBatch.netInstructed)})
        </div>
      </div>

      {/* 5-Step Timeline List */}
      <div className="space-y-4 relative before:absolute before:inset-0 before:left-3.5 before:w-0.5 before:bg-zinc-800 before:z-0">
        {currentBatch.steps.map((s) => (
          <div key={s.step} className="relative z-10 flex items-start gap-4">
            <div
              className={`h-7 w-7 rounded-full flex items-center justify-center text-xs font-bold font-mono shrink-0 shadow-sm ${
                s.status === "COMPLETED"
                  ? "bg-emerald-600 text-white ring-4 ring-zinc-950"
                  : s.status === "IN_PROGRESS"
                  ? "bg-amber-600 text-white ring-4 ring-zinc-950 animate-pulse"
                  : s.status === "FAILED"
                  ? "bg-rose-600 text-white ring-4 ring-zinc-950"
                  : "bg-zinc-800 text-zinc-400 ring-4 ring-zinc-950"
              }`}
            >
              {s.step}
            </div>

            <div className="flex-1 rounded-xl border border-zinc-800 bg-zinc-900/30 p-4 space-y-2">
              <div className="flex items-center justify-between">
                <span className="font-semibold text-sm text-zinc-100 flex items-center gap-2">
                  <span>Step {s.step}: {s.name}</span>
                  <span
                    className={`text-[9px] font-mono px-1.5 py-0.5 rounded font-bold ${
                      s.status === "COMPLETED"
                        ? "bg-emerald-950 text-emerald-400 border border-emerald-800"
                        : s.status === "IN_PROGRESS"
                        ? "bg-amber-950 text-amber-400 border border-amber-800"
                        : s.status === "FAILED"
                        ? "bg-rose-950 text-rose-400 border border-rose-800"
                        : "bg-zinc-800 text-zinc-400"
                    }`}
                  >
                    {s.status}
                  </span>
                </span>
                <span className="text-xs font-mono text-zinc-500">{s.time}</span>
              </div>
              <p className="text-xs text-zinc-300">{s.description}</p>
              <div className="p-2.5 rounded bg-zinc-950 border border-zinc-800/80 font-mono text-[11px] text-zinc-400">
                {s.details}
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
