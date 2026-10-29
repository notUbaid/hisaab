import React from "react";
import { formatPaiseToINR } from "@/lib/rbac";

export default function ExceptionsPage() {
  const exceptions = [
    {
      id: "EXC-20261030-01",
      class: "INSUFFICIENT_BALANCE",
      orderId: "ORD-99120",
      expected: 30000,
      actual: 15000,
      ageingDays: 2,
      status: "OPEN",
      details: "Collector settlement balance (₹150.00) insufficient for matched claims (₹300.00) under all-or-nothing policy (EC-12).",
    },
    {
      id: "EXC-20261030-02",
      class: "AMOUNT_MISMATCH",
      orderId: "ORD-88419",
      expected: 10000,
      actual: 12000,
      ageingDays: 1,
      status: "OPEN",
      details: "Buyer claimed ₹100.00 vs Seller claimed ₹120.00. Exceeds 0 paise tolerance under strict policy (EC-08).",
    },
    {
      id: "EXC-20261030-03",
      class: "BANK_FAILURE",
      orderId: "ORD-77102",
      expected: 8500,
      actual: 0,
      ageingDays: 3,
      status: "PENDING_APPROVAL",
      details: "Bank instruction returned ACCOUNT_FROZEN. Routed to sa:suspense. Maker proposed reversing entry.",
    },
    {
      id: "EXC-20261030-04",
      class: "UNCLAIMED_BANK_CREDIT",
      orderId: "UTR-551029",
      expected: 0,
      actual: 50000,
      ageingDays: 0,
      status: "OPEN",
      details: "Bank credit of ₹500.00 received with unknown reference. No matching SA settlement batch instruction.",
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-white">Reconciliation Exception Queue</h1>
          <p className="text-sm text-zinc-400 mt-1">
            Breaks classified across the 11-class operational taxonomy. Ageing tracked in Indian business days.
          </p>
        </div>
        <div className="flex gap-2">
          <span className="px-3 py-1.5 rounded-lg border border-zinc-800 bg-zinc-900 text-xs font-mono text-zinc-300">
            Open Breaks: <strong className="text-rose-400">3</strong>
          </span>
          <span className="px-3 py-1.5 rounded-lg border border-zinc-800 bg-zinc-900 text-xs font-mono text-zinc-300">
            Pending Maker-Checker: <strong className="text-amber-400">1</strong>
          </span>
        </div>
      </div>

      <div className="rounded-xl border border-zinc-800 bg-zinc-900/30 overflow-hidden">
        <table className="w-full text-left text-sm">
          <thead className="text-xs uppercase text-zinc-400 border-b border-zinc-800 bg-zinc-900/60">
            <tr>
              <th className="py-3 px-4">Exception ID</th>
              <th className="py-3 px-4">Taxonomy Class</th>
              <th className="py-3 px-4">Network Order / Ref</th>
              <th className="py-3 px-4 text-right">Expected</th>
              <th className="py-3 px-4 text-right">Actual</th>
              <th className="py-3 px-4 text-center">Ageing (Biz Days)</th>
              <th className="py-3 px-4 text-center">Status</th>
              <th className="py-3 px-4 text-right">Action</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-800/60 font-mono text-xs">
            {exceptions.map((e) => (
              <tr key={e.id} className="hover:bg-zinc-800/30 transition">
                <td className="py-3.5 px-4 font-medium text-zinc-200">{e.id}</td>
                <td className="py-3.5 px-4">
                  <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-zinc-800 text-zinc-300 border border-zinc-700">
                    {e.class}
                  </span>
                </td>
                <td className="py-3.5 px-4 text-zinc-300">{e.orderId}</td>
                <td className="py-3.5 px-4 text-right text-zinc-400">{formatPaiseToINR(e.expected)}</td>
                <td className="py-3.5 px-4 text-right text-zinc-200 font-semibold">{formatPaiseToINR(e.actual)}</td>
                <td className="py-3.5 px-4 text-center">
                  <span className={`px-2 py-0.5 rounded text-[10px] font-bold ${e.ageingDays >= 2 ? "bg-rose-950 text-rose-400 border border-rose-800" : "text-zinc-400"}`}>
                    {e.ageingDays} d
                  </span>
                </td>
                <td className="py-3.5 px-4 text-center">
                  <span
                    className={`inline-block px-2 py-0.5 rounded text-[10px] font-bold ${
                      e.status === "OPEN"
                        ? "bg-rose-950 text-rose-400 border border-rose-800"
                        : "bg-amber-950 text-amber-400 border border-amber-800"
                    }`}
                  >
                    {e.status}
                  </span>
                </td>
                <td className="py-3.5 px-4 text-right">
                  <button className="px-2.5 py-1 rounded bg-zinc-800 hover:bg-zinc-700 text-zinc-200 text-xs transition">
                    Inspect &rarr;
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
