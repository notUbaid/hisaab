import React from "react";
import Link from "next/link";
import { formatPaiseToINR } from "@/lib/rbac";

export default function DashboardPage() {
  const stats = [
    { label: "Active Invariants", value: "15 / 15", sub: "Formally Enforced & Attacked", status: "green" },
    { label: "BRD Spec Coverage", value: "23 / 23", sub: "100% Executable Scenarios", status: "green" },
    { label: "Settlement Volume", value: formatPaiseToINR(15420000), sub: "Total paise accounted", status: "zinc" },
    { label: "Reconciliation Balance", value: "0.00 paise", sub: "Trial balance delta", status: "green" },
  ];

  const recentBatches = [
    { id: "BATCH-20261030-01", npPair: "bap.dunzo.com:bpp.blinkit.com", status: "SETTLED", orders: 42, volume: 420000 },
    { id: "BATCH-20261030-02", npPair: "bap.paytm.com:bpp.zepto.com", status: "SETTLED", orders: 128, volume: 1280000 },
    { id: "BATCH-20261030-03", npPair: "bap.phonepe.com:bpp.swiggy.com", status: "MATCHING", orders: 18, volume: 180000 },
    { id: "BATCH-20261030-04", npPair: "bap.tata1mg.com:bpp.apollo.com", status: "EXCEPTION", orders: 3, volume: 30000 },
  ];

  return (
    <div className="space-y-8">
      {/* Hero Welcome */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-white">Operations & Correctness Console</h1>
          <p className="text-sm text-zinc-400 mt-1">
            Real-time money-state audit, ONDC RSF 2.0 Beckn flow simulation, and ledger verification.
          </p>
        </div>
        <div className="flex gap-3">
          <Link
            href="/playground"
            className="px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white font-medium text-sm transition shadow-sm"
          >
            Launch Scenario Playground
          </Link>
          <Link
            href="/timeline"
            className="px-4 py-2 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-zinc-200 font-medium text-sm transition"
          >
            Inspect Batch Timeline
          </Link>
        </div>
      </div>

      {/* Metric Cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        {stats.map((s, i) => (
          <div key={i} className="p-5 rounded-xl border border-zinc-800 bg-zinc-900/40">
            <div className="text-xs font-medium text-zinc-400 uppercase tracking-wider">{s.label}</div>
            <div className="text-2xl font-bold text-white mt-2 font-mono">{s.value}</div>
            <div className="text-xs text-zinc-500 mt-1 flex items-center gap-1.5">
              {s.status === "green" && <span className="h-2 w-2 rounded-full bg-emerald-500"></span>}
              {s.sub}
            </div>
          </div>
        ))}
      </div>

      {/* Two-Column Grid: Recent Batches & Active Invariants */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Batches Table */}
        <div className="lg:col-span-2 rounded-xl border border-zinc-800 bg-zinc-900/30 p-6 space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold text-white">Recent Settlement Batches</h2>
            <Link href="/timeline" className="text-xs text-emerald-400 hover:underline">
              View All Batches &rarr;
            </Link>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="text-xs uppercase text-zinc-400 border-b border-zinc-800">
                <tr>
                  <th className="py-2.5 px-3">Batch ID</th>
                  <th className="py-2.5 px-3">NP Pair</th>
                  <th className="py-2.5 px-3 text-right">Orders</th>
                  <th className="py-2.5 px-3 text-right">Volume</th>
                  <th className="py-2.5 px-3 text-center">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-800/60 font-mono text-xs">
                {recentBatches.map((b) => (
                  <tr key={b.id} className="hover:bg-zinc-800/30 transition">
                    <td className="py-3 px-3 font-medium text-zinc-200">{b.id}</td>
                    <td className="py-3 px-3 text-zinc-400">{b.npPair}</td>
                    <td className="py-3 px-3 text-right text-zinc-300">{b.orders}</td>
                    <td className="py-3 px-3 text-right text-zinc-200">{formatPaiseToINR(b.volume)}</td>
                    <td className="py-3 px-3 text-center">
                      <span
                        className={`inline-block px-2 py-0.5 rounded text-[10px] font-bold ${
                          b.status === "SETTLED"
                            ? "bg-emerald-950 text-emerald-400 border border-emerald-800"
                            : b.status === "EXCEPTION"
                            ? "bg-rose-950 text-rose-400 border border-rose-800"
                            : "bg-amber-950 text-amber-400 border border-amber-800"
                        }`}
                      >
                        {b.status}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* Invariants Status Card */}
        <div className="rounded-xl border border-zinc-800 bg-zinc-900/30 p-6 space-y-4">
          <h2 className="text-lg font-semibold text-white">Active Core Invariants</h2>
          <div className="space-y-3 text-xs">
            <div className="p-3 rounded-lg border border-zinc-800 bg-zinc-900/60 flex items-start gap-3">
              <span className="text-emerald-400 font-bold font-mono">01</span>
              <div>
                <div className="font-semibold text-zinc-200">Zero-Sum Conservation</div>
                <div className="text-zinc-500 mt-0.5">Every journal transaction debits == credits in integer paise.</div>
              </div>
            </div>

            <div className="p-3 rounded-lg border border-zinc-800 bg-zinc-900/60 flex items-start gap-3">
              <span className="text-emerald-400 font-bold font-mono">03</span>
              <div>
                <div className="font-semibold text-zinc-200">Effectively-Once Idempotency</div>
                <div className="text-zinc-500 mt-0.5">Insert-first key with SHA-256 payload digest prevents duplicate postings.</div>
              </div>
            </div>

            <div className="p-3 rounded-lg border border-zinc-800 bg-zinc-900/60 flex items-start gap-3">
              <span className="text-emerald-400 font-bold font-mono">04</span>
              <div>
                <div className="font-semibold text-zinc-200">Non-Negative Settlement Balances</div>
                <div className="text-zinc-500 mt-0.5">Constrained accounts locked in ascending ID order; never overdraft.</div>
              </div>
            </div>

            <div className="p-3 rounded-lg border border-zinc-800 bg-zinc-900/60 flex items-start gap-3">
              <span className="text-emerald-400 font-bold font-mono">13</span>
              <div>
                <div className="font-semibold text-zinc-200">Gross-to-Net Tie-Out</div>
                <div className="text-zinc-500 mt-0.5">Order sub-ledger legs tie out to net bank instructions (Total - BFF).</div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
