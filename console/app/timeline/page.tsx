import React from "react";
import { formatPaiseToINR } from "@/lib/rbac";

export default function TimelinePage() {
  const steps = [
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
      details: "42 of 42 orders matched 100%. Total Matched Volume: ₹4,200.00 (420,000 paise). Total BFF: ₹210.00.",
    },
    {
      step: 3,
      name: "Collector Balance Verification (EC-12)",
      time: "18:00:20 IST",
      status: "COMPLETED",
      description: "SA checks Collector ondc_settlement balance against required net transfer.",
      details: "Required Transfer: ₹3,990.00 (Total Matched - BFF). Available Collector Balance: ₹15,000.00. PASSED.",
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
      details: "UTR: HDFC20261030009941. Reversing or modifying entries strictly trigger-blocked. Batch Merkle root computed.",
    },
  ];

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight text-white">Settlement Batch Timeline</h1>
        <p className="text-sm text-zinc-400 mt-1">
          Traceable lifecycle of Batch <code className="text-zinc-200 font-mono">BATCH-20261030-01</code> (bap.dunzo.com &harr; bpp.blinkit.com)
        </p>
      </div>

      {/* Batch Overview Banner */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 p-4 rounded-xl border border-zinc-800 bg-zinc-900/40 font-mono text-xs">
        <div>
          <span className="text-zinc-500 uppercase">Gross Volume:</span>
          <div className="text-zinc-200 font-bold text-sm mt-0.5">{formatPaiseToINR(420000)}</div>
        </div>
        <div>
          <span className="text-zinc-500 uppercase">Buyer Finder Fee (BFF):</span>
          <div className="text-zinc-200 font-bold text-sm mt-0.5">{formatPaiseToINR(21000)}</div>
        </div>
        <div>
          <span className="text-zinc-500 uppercase">Net Instructed:</span>
          <div className="text-emerald-400 font-bold text-sm mt-0.5">{formatPaiseToINR(399000)}</div>
        </div>
        <div>
          <span className="text-zinc-500 uppercase">Terminal Status:</span>
          <div className="text-emerald-400 font-bold text-sm mt-0.5">SETTLED (Verified)</div>
        </div>
      </div>

      {/* Timeline Steps */}
      <div className="relative border-l-2 border-zinc-800 ml-4 space-y-8 py-2">
        {steps.map((s) => (
          <div key={s.step} className="relative pl-6">
            {/* Circle Node */}
            <span className="absolute -left-[9px] top-1 h-4 w-4 rounded-full bg-emerald-500 ring-4 ring-zinc-950 flex items-center justify-center">
              <span className="h-1.5 w-1.5 rounded-full bg-zinc-950"></span>
            </span>

            <div className="rounded-xl border border-zinc-800 bg-zinc-900/30 p-5 space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-sm font-semibold text-white flex items-center gap-2">
                  <span className="font-mono text-xs text-emerald-400">Step {s.step}:</span>
                  {s.name}
                </span>
                <span className="text-xs font-mono text-zinc-500">{s.time}</span>
              </div>
              <p className="text-xs text-zinc-300">{s.description}</p>
              <div className="mt-2 p-3 rounded-lg bg-zinc-950/60 border border-zinc-800/80 font-mono text-xs text-zinc-400">
                {s.details}
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
