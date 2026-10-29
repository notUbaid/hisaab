"use client";

import React, { useState } from "react";
import { formatPaiseToINR } from "@/lib/rbac";

interface ExceptionItem {
  id: string;
  class: string;
  orderId: string;
  expected: number;
  actual: number;
  ageingDays: number;
  status: "OPEN" | "PENDING_APPROVAL" | "RESOLVED";
  details: string;
  proposedBy?: string;
  approvedBy?: string;
  adjustmentType?: string;
}

const INITIAL_EXCEPTIONS: ExceptionItem[] = [
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
    proposedBy: "maker.ops@nodalbank.in",
    adjustmentType: "REVERSING_ENTRY",
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

export default function ExceptionsPage() {
  const [exceptions, setExceptions] = useState<ExceptionItem[]>(INITIAL_EXCEPTIONS);
  const [selectedException, setSelectedException] = useState<ExceptionItem | null>(null);

  // Maker-Checker Form State
  const [makerId, setMakerId] = useState<string>("maker.finance@np.org");
  const [adjustmentType, setAdjustmentType] = useState<string>("REVERSING_ENTRY");
  const [checkerId, setCheckerId] = useState<string>("checker.supervisor@np.org");
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const handleOpenInspect = (item: ExceptionItem) => {
    setSelectedException(item);
    setErrorMsg(null);
    setSuccessMsg(null);
  };

  const handleProposeAdjustment = (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedException) return;

    if (!makerId.trim()) {
      setErrorMsg("Maker User ID is required");
      return;
    }

    setExceptions((prev) =>
      prev.map((item) =>
        item.id === selectedException.id
          ? {
              ...item,
              status: "PENDING_APPROVAL",
              proposedBy: makerId,
              adjustmentType: adjustmentType,
            }
          : item
      )
    );

    setSelectedException((prev) =>
      prev
        ? {
            ...prev,
            status: "PENDING_APPROVAL",
            proposedBy: makerId,
            adjustmentType: adjustmentType,
          }
        : null
    );

    setSuccessMsg(`Adjustment proposed by ${makerId}. Awaiting dual-control Checker approval.`);
    setErrorMsg(null);
  };

  const handleApproveAdjustment = () => {
    if (!selectedException) return;

    // Enforce Dual-Control Invariant: approver cannot be the same as proposer
    if (checkerId.trim().toLowerCase() === selectedException.proposedBy?.trim().toLowerCase()) {
      setErrorMsg("DUAL CONTROL VIOLATION: Approver cannot be identical to Proposer (Self-Approval Prohibited).");
      return;
    }

    setExceptions((prev) =>
      prev.map((item) =>
        item.id === selectedException.id
          ? {
              ...item,
              status: "RESOLVED",
              approvedBy: checkerId,
            }
          : item
      )
    );

    setSelectedException((prev) =>
      prev
        ? {
            ...prev,
            status: "RESOLVED",
            approvedBy: checkerId,
          }
        : null
    );

    setSuccessMsg(`Approved by ${checkerId}. Reversing entry posted to immutable ledger.`);
    setErrorMsg(null);
  };

  const openCount = exceptions.filter((e) => e.status === "OPEN").length;
  const pendingCount = exceptions.filter((e) => e.status === "PENDING_APPROVAL").length;
  const resolvedCount = exceptions.filter((e) => e.status === "RESOLVED").length;

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-white">Reconciliation Exception Queue</h1>
          <p className="text-sm text-zinc-400 mt-1">
            Breaks classified across the 11-class operational taxonomy. Ageing tracked in Indian business days.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <span className="px-3 py-1.5 rounded-lg border border-zinc-800 bg-zinc-900 text-xs font-mono text-zinc-300">
            Open: <strong className="text-rose-400">{openCount}</strong>
          </span>
          <span className="px-3 py-1.5 rounded-lg border border-zinc-800 bg-zinc-900 text-xs font-mono text-zinc-300">
            Pending Dual Control: <strong className="text-amber-400">{pendingCount}</strong>
          </span>
          <span className="px-3 py-1.5 rounded-lg border border-zinc-800 bg-zinc-900 text-xs font-mono text-zinc-300">
            Resolved: <strong className="text-emerald-400">{resolvedCount}</strong>
          </span>
        </div>
      </div>

      <div className="rounded-xl border border-zinc-800 bg-zinc-900/30 overflow-hidden">
        <table className="w-full text-left text-sm">
          <thead className="text-xs uppercase text-zinc-400 border-b border-zinc-800 bg-zinc-900/60 font-semibold tracking-wider">
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
                  <span
                    className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                      e.ageingDays >= 2
                        ? "bg-rose-950 text-rose-400 border border-rose-800"
                        : "text-zinc-400"
                    }`}
                  >
                    {e.ageingDays} d
                  </span>
                </td>
                <td className="py-3.5 px-4 text-center">
                  <span
                    className={`inline-block px-2 py-0.5 rounded text-[10px] font-bold ${
                      e.status === "OPEN"
                        ? "bg-rose-950 text-rose-400 border border-rose-800"
                        : e.status === "PENDING_APPROVAL"
                        ? "bg-amber-950 text-amber-400 border border-amber-800"
                        : "bg-emerald-950 text-emerald-400 border border-emerald-800"
                    }`}
                  >
                    {e.status}
                  </span>
                </td>
                <td className="py-3.5 px-4 text-right">
                  <button
                    onClick={() => handleOpenInspect(e)}
                    className="px-2.5 py-1 rounded bg-zinc-800 hover:bg-zinc-700 text-zinc-200 text-xs transition cursor-pointer"
                  >
                    Inspect &rarr;
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Interactive Maker-Checker Resolution Drawer / Modal */}
      {selectedException && (
        <div className="rounded-xl border border-zinc-700 bg-zinc-900/90 p-6 space-y-5 animate-in fade-in duration-150">
          <div className="flex items-center justify-between border-b border-zinc-800 pb-4">
            <div>
              <div className="flex items-center gap-2 font-mono text-xs">
                <span className="font-bold text-emerald-400">{selectedException.id}</span>
                <span className="text-zinc-500">&bull;</span>
                <span className="px-2 py-0.5 rounded bg-zinc-800 text-zinc-300 font-mono text-[10px]">
                  {selectedException.class}
                </span>
              </div>
              <h2 className="text-lg font-bold text-white mt-1">Resolution & Maker-Checker Workflow</h2>
            </div>
            <button
              onClick={() => setSelectedException(null)}
              className="text-zinc-400 hover:text-white text-xs px-2.5 py-1 rounded border border-zinc-800 bg-zinc-800/50"
            >
              Close
            </button>
          </div>

          <div className="p-4 rounded-lg bg-zinc-950 border border-zinc-800/80 text-xs space-y-1 font-mono">
            <div className="text-zinc-500 uppercase tracking-wider text-[10px]">Exception Details:</div>
            <div className="text-zinc-200 leading-relaxed">{selectedException.details}</div>
            <div className="pt-2 flex gap-4 text-zinc-400 text-[11px]">
              <span>Order ID: <strong className="text-zinc-200">{selectedException.orderId}</strong></span>
              <span>Expected: <strong className="text-zinc-200">{formatPaiseToINR(selectedException.expected)}</strong></span>
              <span>Actual: <strong className="text-zinc-200">{formatPaiseToINR(selectedException.actual)}</strong></span>
              <span>Ageing: <strong className="text-rose-400">{selectedException.ageingDays} Indian biz days</strong></span>
            </div>
          </div>

          {errorMsg && (
            <div className="p-3 rounded-lg bg-rose-950/80 border border-rose-800 text-rose-300 text-xs font-mono">
              ⚠️ {errorMsg}
            </div>
          )}

          {successMsg && (
            <div className="p-3 rounded-lg bg-emerald-950/80 border border-emerald-800 text-emerald-300 text-xs font-mono">
              ✓ {successMsg}
            </div>
          )}

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6 pt-2">
            {/* Step 1: Maker Role */}
            <div className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-4 space-y-3">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold text-zinc-300 uppercase tracking-wider">Role 1: Maker</span>
                {selectedException.proposedBy && (
                  <span className="text-[10px] text-emerald-400 font-mono">✓ Proposed</span>
                )}
              </div>
              <p className="text-[11px] text-zinc-400">
                Propose a corrective reversing journal entry or suspense reclassification.
              </p>

              <form onSubmit={handleProposeAdjustment} className="space-y-3 font-mono text-xs">
                <div>
                  <label htmlFor="maker-user-id" className="block text-[10px] text-zinc-500 uppercase mb-1">Maker User ID:</label>
                  <input
                    id="maker-user-id"
                    type="text"
                    value={makerId}
                    onChange={(e) => setMakerId(e.target.value)}
                    disabled={selectedException.status !== "OPEN"}
                    className="w-full px-2.5 py-1.5 rounded bg-zinc-900 border border-zinc-800 text-zinc-200 disabled:opacity-50"
                  />
                </div>
                <div>
                  <label className="block text-[10px] text-zinc-500 uppercase mb-1">Adjustment Action:</label>
                  <select
                    value={adjustmentType}
                    onChange={(e) => setAdjustmentType(e.target.value)}
                    disabled={selectedException.status !== "OPEN"}
                    className="w-full px-2.5 py-1.5 rounded bg-zinc-900 border border-zinc-800 text-zinc-200 disabled:opacity-50"
                  >
                    <option value="REVERSING_ENTRY">Post Reversing Journal Entry</option>
                    <option value="SUSPENSE_CLEARING">Transfer to SA Suspense Account</option>
                    <option value="FORCE_MAJEURE_CLAWBACK">Schedule Next-Batch Clawback</option>
                  </select>
                </div>

                {selectedException.status === "OPEN" && (
                  <button
                    type="submit"
                    className="w-full py-2 rounded bg-zinc-800 hover:bg-zinc-700 text-white font-medium text-xs transition cursor-pointer"
                  >
                    Submit Proposal for Review &rarr;
                  </button>
                )}
              </form>
            </div>

            {/* Step 2: Checker Role */}
            <div className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-4 space-y-3">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold text-zinc-300 uppercase tracking-wider">Role 2: Checker (Dual Control)</span>
                {selectedException.approvedBy && (
                  <span className="text-[10px] text-emerald-400 font-mono">✓ Approved</span>
                )}
              </div>
              <p className="text-[11px] text-zinc-400">
                Independent reviewer must approve. Cannot be identical to the proposing Maker.
              </p>

              <div className="space-y-3 font-mono text-xs">
                <div>
                  <label htmlFor="checker-user-id" className="block text-[10px] text-zinc-500 uppercase mb-1">Checker User ID:</label>
                  <input
                    id="checker-user-id"
                    type="text"
                    value={checkerId}
                    onChange={(e) => setCheckerId(e.target.value)}
                    disabled={selectedException.status !== "PENDING_APPROVAL"}
                    className="w-full px-2.5 py-1.5 rounded bg-zinc-900 border border-zinc-800 text-zinc-200 disabled:opacity-50"
                  />
                </div>

                <div className="p-2.5 rounded bg-zinc-900/80 border border-zinc-800 text-[10px] text-zinc-400">
                  Dual Control Rule: Approver (<code className="text-zinc-200">{checkerId}</code>) must differ from Maker (<code className="text-zinc-200">{selectedException.proposedBy || makerId}</code>).
                </div>

                {selectedException.status === "PENDING_APPROVAL" && (
                  <button
                    type="button"
                    onClick={handleApproveAdjustment}
                    className="w-full py-2 rounded bg-emerald-600 hover:bg-emerald-500 text-white font-medium text-xs transition cursor-pointer"
                  >
                    Authorize & Post to Ledger &rarr;
                  </button>
                )}

                {selectedException.status === "RESOLVED" && (
                  <div className="py-2 text-center text-xs text-emerald-400 font-semibold bg-emerald-950/40 rounded border border-emerald-900">
                    ✓ Resolved & Reconciled
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
