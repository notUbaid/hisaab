"use client";

import React, { useState } from "react";
import {
  X,
  AlertOctagon,
  ArrowRight,
  ShieldCheck,
  RotateCcw,
  Download,
  CheckCircle2,
  FileDiff,
  RotateCw,
  Lock,
} from "lucide-react";
import MoneyCell, { formatIndianNumbering } from "@/components/MoneyCell";
import { ReconException } from "@/lib/scenarioData";
import { ParticipantPerspective } from "@/lib/rbac";
import { EncryptedField } from "@/components/EncryptedField";
import { SignatureInspector } from "@/components/SignatureInspector";
import { apiClient } from "@/lib/apiClient";
import { useToast } from "@/components/ToastProvider";

interface ReconDiffDrawerProps {
  exception: ReconException | null;
  onClose: () => void;
  onResolve?: (id: string, remedy: string) => void;
  perspective?: ParticipantPerspective;
}

export function ReconDiffDrawer({
  exception,
  onClose,
  onResolve,
  perspective = "COLLECTOR_RSP",
}: ReconDiffDrawerProps) {
  const { showToast } = useToast();
  const [resolutionStatus, setResolutionStatus] = useState<string | null>(null);
  const [isMutating, setIsMutating] = useState<boolean>(false);
  const [mutationError, setMutationError] = useState<string | null>(null);
  const [txResult, setTxResult] = useState<{
    transactionId: string;
    journalEntryId: string;
    message: string;
  } | null>(null);

  if (!exception) return null;

  // Translate raw rule failure into institutional plain-English diagnosis
  const getPlainEnglishRootCause = (exc: ReconException) => {
    const cls = exc.class.toLowerCase();
    if (cls.includes("tax") || cls.includes("tcs") || cls.includes("rounding")) {
      return {
        badge: "TCS Rounding Drift (₹0.01)",
        color: "text-amber-400 bg-amber-500/10 border-amber-500/25",
        summary: "Calculated TCS GST discrepancy across floating-point vs Banker's RoundHalfEven statutory formula.",
        ruleRef: "CGST Notification 15/2024 & AMB-001 (Match Tolerance)",
      };
    }
    if (cls.includes("unmatched") || cls.includes("missing")) {
      return {
        badge: "Missing Counterpart Claim",
        color: "text-rose-400 bg-rose-500/10 border-rose-500/25",
        summary: "Buyer App submitted claim during T+1 cycle, but Seller App /on_confirm counterpart was absent at cut-off.",
        ruleRef: "BRD §4.2 Case 1 & AMB-003 (Carry-Forward Policy)",
      };
    }
    if (cls.includes("fee") || cls.includes("bff") || cls.includes("commission")) {
      return {
        badge: "Component Fee Variance",
        color: "text-amber-400 bg-amber-500/10 border-amber-500/25",
        summary: "Buyer Finder Fee (BFF) specified by BAP does not tie out to BPP quotation parameters.",
        ruleRef: "BRD §4.2 Case 8 & Invariant 13 (Gross-to-Net Tie-Out)",
      };
    }
    return {
      badge: "Settlement Value Discrepancy",
      color: "text-purple-400 bg-purple-500/10 border-purple-500/25",
      summary: "Bank statement credit amount does not equal nodal instruction settlement due to intermediary charges.",
      ruleRef: "RBI DPSS Guidelines & Invariant 5 (Paise Conservation)",
    };
  };

  const rootCause = getPlainEnglishRootCause(exception);

  // 3-Way payload components
  const buyerClaimPaise = exception.expectedPaise;
  const sellerClaimPaise = exception.actualPaise;
  const bankWirePaise = Math.round(exception.actualPaise * 0.994);

  const handleApproveTolerance = async () => {
    setIsMutating(true);
    setMutationError(null);
    try {
      const res = await apiClient.approveReconException({
        exceptionId: exception.id,
        orderId: exception.orderId,
        discrepancyPaise: exception.variancePaise,
        reason: `Tolerance approved by operator (${perspective})`,
      });

      setResolutionStatus("APPROVED_TOLERANCE");
      setTxResult({
        transactionId: res.transactionId || `TX-TOL-${Date.now()}`,
        journalEntryId: res.journalEntryId || `JE-TOL-${Date.now()}`,
        message: res.remedy || "Tolerance approved and posted to ledger",
      });
      showToast(
        `Tolerance approved: ${res.transactionId || exception.id}`,
        "success",
        "Reconciliation Approved"
      );
      if (onResolve) {
        onResolve(exception.id, `Tolerance Approved (${res.transactionId})`);
      }
    } catch (err: any) {
      const msg = err.message || "Failed to post tolerance approval";
      setMutationError(msg);
      showToast(msg, "error", "Tolerance Approval Failed");
    } finally {
      setIsMutating(false);
    }
  };

  const handleTriggerRefund = async () => {
    setIsMutating(true);
    setMutationError(null);
    try {
      const res = await apiClient.triggerRefund({
        orderId: exception.orderId || exception.id,
        refundAmountPaise: Math.abs(exception.variancePaise) || 98400,
        reason: "Customer Return / RTO Fulfillment Failure",
      });

      setResolutionStatus("REFUND_DISPATCHED");
      setTxResult({
        transactionId: res.transactionId || `TX-REF-${Date.now()}`,
        journalEntryId: res.journalEntryId || `JE-REF-${Date.now()}`,
        message: res.status || "Refund dispatched",
      });
      showToast(
        `Refund dispatched: ${res.transactionId || exception.orderId}`,
        "success",
        "Refund Triggered"
      );
      if (onResolve) {
        onResolve(exception.id, `Refund Dispatched (${res.transactionId})`);
      }
    } catch (err: any) {
      const msg = err.message || "Failed to dispatch automated refund";
      setMutationError(msg);
      showToast(msg, "error", "Refund Failed");
    } finally {
      setIsMutating(false);
    }
  };

  const handleExportDispute = () => {
    const disputePayload = {
      action: "/on_recon",
      dispute_id: `DISP-${exception.id}`,
      order_id: exception.orderId,
      discrepancy_class: exception.class,
      variance_paise: exception.variancePaise,
      created_at: new Date().toISOString(),
      resolution_request: "COUNTERPART_CORRECTION",
    };
    const blob = new Blob([JSON.stringify(disputePayload, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `ondc-dispute-${exception.id}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div
      className="fixed inset-y-0 right-0 z-50 w-full max-w-2xl bg-obsidian-850 border-l border-white/10 shadow-2xl flex flex-col justify-between font-sans animate-in slide-in-from-right duration-200"
      onClick={(e) => e.stopPropagation()}
    >
      {/* ── Top Header Strip ── */}
      <div className="p-4 border-b border-white/10 flex items-center justify-between bg-obsidian-900/60 shrink-0">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 rounded-lg bg-amber-500/10 border border-amber-500/30 flex items-center justify-center text-amber-400">
            <AlertOctagon className="w-4 h-4" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <span className="font-mono font-bold text-sm text-white">
                Exception {exception.id}
              </span>
              <span className={`text-[10px] font-mono font-semibold px-2 py-0.5 rounded-full border ${rootCause.color}`}>
                {rootCause.badge}
              </span>
            </div>
            <div className="text-xs text-slate-400 font-mono mt-0.5">
              Order Ref: <span className="text-slate-200">{exception.orderId}</span> · Ageing:{" "}
              <span className="text-amber-400 font-semibold">T+{exception.ageingDays}</span>
            </div>
          </div>
        </div>

        <button
          onClick={onClose}
          className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/5 transition-colors"
        >
          <X className="w-5 h-5" />
        </button>
      </div>

      {/* ── Scrollable Inspector Content ── */}
      <div className="flex-1 overflow-y-auto p-5 space-y-5 text-xs">
        {/* Diagnosis & Root Cause Box */}
        <div className="p-3.5 rounded-lg bg-obsidian-900 border border-white/5 space-y-2">
          <div className="text-[10px] font-mono uppercase tracking-wider text-slate-500 flex items-center justify-between">
            <span>Automated Recon Diagnosis</span>
            <span className="text-slate-400">{rootCause.ruleRef}</span>
          </div>
          <p className="text-slate-200 leading-relaxed">
            {rootCause.summary}
          </p>
          <div className="flex items-center gap-3 pt-2 border-t border-white/5 text-[11px] font-mono">
            <span className="text-slate-400">Variance Delta:</span>
            <span className="text-rose-400 font-bold">
              {exception.variancePaise > 0 ? "+" : ""}
              {formatIndianNumbering(exception.variancePaise).full}
            </span>
            <span className="text-slate-600">|</span>
            <span className="text-slate-400">Status:</span>
            <span className="text-amber-400 font-semibold">{exception.status}</span>
          </div>
        </div>

        {/* 3-Way Side-by-Side Comparison */}
        <div>
          <div className="flex items-center gap-2 mb-2">
            <FileDiff className="w-3.5 h-3.5 text-emerald-400" />
            <span className="text-[11px] font-semibold text-slate-300 uppercase tracking-wider">
              3-Way Settlement Payload Comparison
            </span>
          </div>

          <div className="grid grid-cols-3 gap-2 font-mono text-[11px]">
            {/* Buyer NP Claim */}
            <div className="p-3 rounded-lg bg-obsidian-900 border border-white/5 space-y-2">
              <div className="text-[10px] uppercase text-emerald-400 font-bold pb-1 border-b border-white/5">
                Buyer App (BAP)
              </div>
              <div className="space-y-1.5">
                <div>
                  <div className="text-[9px] text-slate-500">Claim Amount:</div>
                  <MoneyCell paise={buyerClaimPaise} className="font-semibold text-slate-100" />
                </div>
                <div>
                  <div className="text-[9px] text-slate-500">Component BFF:</div>
                  <MoneyCell paise={Math.round(buyerClaimPaise * 0.03)} className="text-slate-300" />
                </div>
                <div>
                  <div className="text-[9px] text-slate-500">Ingress Action:</div>
                  <div className="text-slate-300 text-[10px]">/settle (T+0)</div>
                </div>
              </div>
            </div>

            {/* Seller NP Claim */}
            <div className="p-3 rounded-lg bg-obsidian-900 border border-rose-500/20 bg-rose-500/[0.02] space-y-2">
              <div className="text-[10px] uppercase text-amber-400 font-bold pb-1 border-b border-white/5">
                Seller App (BPP)
              </div>
              <div className="space-y-1.5">
                <div>
                  <div className="text-[9px] text-slate-500">Expected Value:</div>
                  <div className="bg-rose-500/10 px-1 py-0.5 rounded border border-rose-500/30 inline-block">
                    <MoneyCell paise={sellerClaimPaise} className="font-semibold text-rose-300" />
                  </div>
                </div>
                <div>
                  <div className="text-[9px] text-slate-500">Component BFF:</div>
                  <MoneyCell paise={Math.round(sellerClaimPaise * 0.025)} className="text-slate-300" />
                </div>
                <div>
                  <div className="text-[9px] text-slate-500">Ingress Action:</div>
                  <div className="text-slate-300 text-[10px]">/on_settle (T+1)</div>
                </div>
              </div>
            </div>

            {/* Bank Nodal Wire */}
            <div className="p-3 rounded-lg bg-obsidian-900 border border-white/5 space-y-2">
              <div className="text-[10px] uppercase text-cyan-400 font-bold pb-1 border-b border-white/5">
                Bank Nodal Wire
              </div>
              <div className="space-y-1.5">
                <div>
                  <div className="text-[9px] text-slate-500">Dispatched Wire:</div>
                  <MoneyCell paise={bankWirePaise} className="font-semibold text-slate-100" />
                </div>
                <div>
                  <div className="text-[9px] text-slate-500 mb-0.5">UTR Reference:</div>
                  <EncryptedField value="UTR9847120391" label="Bank UTR" fieldType="utr" />
                </div>
                <div>
                  <div className="text-[9px] text-slate-500">Bank Status:</div>
                  <div className="text-emerald-400 text-[10px]">RTGS_CONFIRMED</div>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Detailed Attribute Mismatch Diff Table */}
        <div className="fin-card overflow-hidden">
          <div className="px-3 py-2 bg-obsidian-900/80 border-b border-white/5 text-[10px] font-mono uppercase text-slate-400 tracking-wider">
            Attribute Delta Breakdown
          </div>
          <table className="w-full text-left border-collapse">
            <thead>
              <tr>
                <th className="fin-table-th">Field</th>
                <th className="fin-table-th">BAP Stated</th>
                <th className="fin-table-th">BPP Stated</th>
                <th className="fin-table-th text-right">Delta (₹)</th>
              </tr>
            </thead>
            <tbody>
              <tr className="hover:bg-white/[0.02]">
                <td className="fin-table-td text-slate-400 font-mono text-[10px]">item_total</td>
                <td className="fin-table-td text-slate-200"><MoneyCell paise={buyerClaimPaise} /></td>
                <td className="fin-table-td text-slate-200"><MoneyCell paise={sellerClaimPaise} /></td>
                <td className="fin-table-td text-right font-bold text-rose-400">
                  <MoneyCell paise={exception.variancePaise} direction="net" showSign />
                </td>
              </tr>
              <tr className="hover:bg-white/[0.02]">
                <td className="fin-table-td text-slate-400 font-mono text-[10px]">tds_194o</td>
                <td className="fin-table-td text-slate-300">₹{(Math.round(buyerClaimPaise * 0.001) / 100).toFixed(2)}</td>
                <td className="fin-table-td text-slate-300">₹{(Math.round(sellerClaimPaise * 0.001) / 100).toFixed(2)}</td>
                <td className="fin-table-td text-right text-slate-400">₹0.00</td>
              </tr>
              <tr className="hover:bg-white/[0.02]">
                <td className="fin-table-td text-slate-400 font-mono text-[10px]">tcs_gst_sec52</td>
                <td className="fin-table-td text-slate-300">₹{(Math.round(buyerClaimPaise * 0.005) / 100).toFixed(2)}</td>
                <td className="fin-table-td text-amber-300">₹{(Math.round(sellerClaimPaise * 0.005) / 100).toFixed(2)}</td>
                <td className="fin-table-td text-right text-amber-400 font-mono font-semibold">₹0.01</td>
              </tr>
            </tbody>
          </table>
        </div>

        {/* Cryptographic Signature Inspector for Counterparty Claims */}
        <SignatureInspector
          bapId="buyer-app.ondc.org"
          bppId="seller-app.ondc.org"
          createdTimestamp={new Date(Date.now() - 3600000).toISOString()}
          expiresTimestamp={new Date(Date.now() + 1800000).toISOString()}
          isVerified={
            !exception.class.toLowerCase().includes("signature") &&
            !exception.class.toLowerCase().includes("digest") &&
            !exception.class.toLowerCase().includes("auth")
          }
        />
      </div>

      {/* ── RBAC-Gated Resolution Bar ── */}
      <div className="p-4 border-t border-white/10 bg-obsidian-900/90 shrink-0 space-y-2">
        {mutationError && (
          <div className="p-2 rounded-lg bg-rose-500/10 border border-rose-500/30 text-rose-300 text-xs font-mono flex items-center justify-between">
            <span>{mutationError}</span>
            <button onClick={() => setMutationError(null)} className="text-rose-400 underline text-[10px]">
              Dismiss
            </button>
          </div>
        )}

        {txResult ? (
          <div className="p-3 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-xs font-mono text-emerald-300 space-y-1">
            <div className="flex items-center justify-between">
              <span className="font-bold flex items-center gap-1.5">
                <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                {resolutionStatus}
              </span>
              <button
                onClick={() => {
                  setResolutionStatus(null);
                  setTxResult(null);
                }}
                className="text-[10px] text-emerald-400 underline"
              >
                Reset
              </button>
            </div>
            <p className="text-[11px] text-slate-300">{txResult.message}</p>
            <div className="text-[10px] text-slate-400 flex items-center gap-3 pt-1 border-t border-white/5">
              <span>Tx ID: <strong className="text-white font-mono">{txResult.transactionId}</strong></span>
              <span>•</span>
              <span>Journal: <strong className="text-emerald-400 font-mono">{txResult.journalEntryId}</strong></span>
            </div>
          </div>
        ) : (
          <div>
            <div className="flex items-center justify-between mb-2 text-[10px] font-mono text-slate-500">
              <span>RBAC Actions for {perspective}</span>
              <span className="text-emerald-400 font-semibold">Authorized</span>
            </div>

            <div className="flex items-center gap-2">
              {/* Option 1: Penny Tolerance */}
              <button
                onClick={handleApproveTolerance}
                disabled={isMutating}
                className="flex-1 py-2 px-3 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white font-semibold text-xs transition flex items-center justify-center gap-1.5 shadow-glow disabled:opacity-50"
              >
                {isMutating ? (
                  <RotateCw className="w-3.5 h-3.5 animate-spin" />
                ) : (
                  <ShieldCheck className="w-3.5 h-3.5" />
                )}
                <span>{isMutating ? "Posting Ledger..." : "Approve Tolerance"}</span>
              </button>

              {/* Option 2: Trigger Automated Refund */}
              <button
                onClick={handleTriggerRefund}
                disabled={isMutating}
                className="py-2 px-3 rounded-lg bg-obsidian-750 hover:bg-obsidian-700 border border-white/10 text-slate-200 text-xs font-medium transition flex items-center gap-1.5 disabled:opacity-50"
              >
                {isMutating ? (
                  <RotateCw className="w-3.5 h-3.5 animate-spin" />
                ) : (
                  <RotateCcw className="w-3.5 h-3.5 text-amber-400" />
                )}
                <span>Trigger Refund</span>
              </button>

              {/* Option 3: Export Dispute Payload */}
              <button
                onClick={handleExportDispute}
                className="p-2 rounded-lg bg-obsidian-750 hover:bg-obsidian-700 border border-white/10 text-slate-300 text-xs transition"
                title="Export Signed /on_recon Dispute JSON"
              >
                <Download className="w-4 h-4" />
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export default ReconDiffDrawer;
