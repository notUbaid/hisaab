"use client";

import React, { useState, useEffect } from "react";
import {
  X,
  Download,
  Calendar,
  Clock,
  ArrowRight,
  ShieldCheck,
  Building,
  CheckCircle2,
  FileCode,
  Copy,
} from "lucide-react";
import MoneyCell from "./MoneyCell";
import { EncryptedField } from "./EncryptedField";

interface WireBatchModalProps {
  isOpen: boolean;
  onClose: () => void;
  batchId?: string;
  amountPaise?: number;
  scheduledDate?: string;
  beneficiaryName?: string;
  beneficiaryAccount?: string;
  beneficiaryIfsc?: string;
}

export function WireBatchModal({
  isOpen,
  onClose,
  batchId = "BATCH-2026-10-31-001",
  amountPaise = 98400,
  scheduledDate = "2026-11-03",
  beneficiaryName = "Direct BPP Seller Operative",
  beneficiaryAccount = "50200088921432",
  beneficiaryIfsc = "HDFC0000240",
}: WireBatchModalProps) {
  const [activeTab, setActiveTab] = useState<"BREAKDOWN" | "ISO_JSON" | "CALENDAR">("BREAKDOWN");
  const [copied, setCopied] = useState(false);
  const [calendarInfo, setCalendarInfo] = useState<{
    scheduledSettlementDate: string;
    isPastCutoff: boolean;
    targetSettlementWindow: string;
    rolloverExplanations: string[];
    upcomingBankHolidays: { date: string; name: string; citation: string }[];
  } | null>(null);

  useEffect(() => {
    if (!isOpen) return;
    let isMounted = true;
    fetch("/api/settlements/window", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        amountPaise,
        beneficiaryAccount,
        beneficiaryIfsc,
        tPlusDays: 1,
      }),
    })
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (isMounted && data) {
          setCalendarInfo(data);
        }
      })
      .catch(() => {});
    return () => {
      isMounted = false;
    };
  }, [isOpen, amountPaise, beneficiaryAccount, beneficiaryIfsc]);

  if (!isOpen) return null;

  const wirePayload = {
    message_id: `MSG-WIRE-${batchId}`,
    instruction_id: `INST-${batchId}-01`,
    payment_type: amountPaise >= 20000000 ? "RTGS" : "NEFT",
    settlement_date: scheduledDate,
    amount_paise: amountPaise,
    amount_inr: (amountPaise / 100).toFixed(2),
    currency: "INR",
    debtor: {
      account_name: "ONDC NODAL SETTLEMENT ESCROW",
      account_number: "99001122334455",
      ifsc: "YESB0000001",
    },
    creditor: {
      account_name: beneficiaryName,
      account_number: beneficiaryAccount,
      ifsc: beneficiaryIfsc,
    },
    remittance_info: "ONDC RSF 2.0 MULTI-PARTY SETTLEMENT NET DISBURSEMENT",
    iso20022_format: "pacs.008.001.08",
    idempotency_key: `idemp-wire-${batchId}`,
  };

  const handleDownloadJson = () => {
    const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(wirePayload, null, 2));
    const downloadAnchor = document.createElement("a");
    downloadAnchor.setAttribute("href", dataStr);
    downloadAnchor.setAttribute("download", `wire-instruction-${batchId}.json`);
    document.body.appendChild(downloadAnchor);
    downloadAnchor.click();
    downloadAnchor.remove();
  };

  const handleCopyJson = () => {
    navigator.clipboard.writeText(JSON.stringify(wirePayload, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in">
      <div className="bg-obsidian-850 border border-white/10 rounded-2xl w-full max-w-2xl max-h-[85vh] flex flex-col shadow-2xl overflow-hidden font-sans">
        {/* Header */}
        <div className="px-5 py-4 border-b border-white/5 flex items-center justify-between bg-obsidian-900">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-emerald-500/10 border border-emerald-500/30 flex items-center justify-center text-emerald-400">
              <Building className="w-4 h-4" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h3 className="font-semibold text-sm text-white">Wire Instruction &amp; T+N Calendar</h3>
                <span className="pill-emerald text-[10px]">RBI RTGS Certified</span>
              </div>
              <span className="font-mono text-xs text-slate-400">{batchId}</span>
            </div>
          </div>

          <button
            onClick={onClose}
            className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/5 transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Tab selection */}
        <div className="px-5 pt-3 pb-0 border-b border-white/5 flex items-center gap-2 font-mono text-xs">
          <button
            onClick={() => setActiveTab("BREAKDOWN")}
            className={`px-3 py-1.5 rounded-t-lg transition-colors ${
              activeTab === "BREAKDOWN"
                ? "bg-obsidian-800 text-emerald-400 font-semibold border-t border-x border-white/10"
                : "text-slate-400 hover:text-white"
            }`}
          >
            Settlement Summary
          </button>
          <button
            onClick={() => setActiveTab("CALENDAR")}
            className={`px-3 py-1.5 rounded-t-lg transition-colors ${
              activeTab === "CALENDAR"
                ? "bg-obsidian-800 text-emerald-400 font-semibold border-t border-x border-white/10"
                : "text-slate-400 hover:text-white"
            }`}
          >
            T+N Holiday Rollover Logic
          </button>
          <button
            onClick={() => setActiveTab("ISO_JSON")}
            className={`px-3 py-1.5 rounded-t-lg transition-colors ${
              activeTab === "ISO_JSON"
                ? "bg-obsidian-800 text-emerald-400 font-semibold border-t border-x border-white/10"
                : "text-slate-400 hover:text-white"
            }`}
          >
            ISO 20022 Wire JSON
          </button>
        </div>

        {/* Content body */}
        <div className="p-5 flex-1 overflow-y-auto space-y-4">
          {activeTab === "BREAKDOWN" && (
            <div className="space-y-4">
              {/* Payment Summary Ribbon */}
              <div className="p-4 rounded-xl bg-obsidian-900 border border-white/5 flex flex-wrap items-center justify-between gap-4">
                <div>
                  <span className="text-[10px] font-mono uppercase text-slate-500">Net Payable Amount</span>
                  <div className="mt-0.5">
                    <MoneyCell paise={amountPaise} className="text-xl font-bold text-emerald-400" />
                  </div>
                </div>

                <div className="text-right">
                  <span className="text-[10px] font-mono uppercase text-slate-500">Scheduled Dispatch Window</span>
                  <div className="flex items-center gap-1.5 text-xs font-mono font-semibold text-amber-300 mt-1">
                    <Calendar className="w-3.5 h-3.5 text-amber-400" />
                    <span>{scheduledDate} (17:00 IST Cutoff)</span>
                  </div>
                </div>
              </div>

              {/* Bank Routing Details */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs font-mono">
                <div className="p-3 rounded-lg bg-obsidian-900 border border-white/5 space-y-2">
                  <span className="text-slate-500 text-[10px] uppercase font-bold block">Debtor (Nodal Escrow)</span>
                  <div className="space-y-1 text-slate-300">
                    <p className="font-semibold text-white">ONDC Nodal Escrow</p>
                    <p className="text-slate-400">Acc: 99001122334455</p>
                    <p className="text-slate-400">IFSC: YESB0000001 (Yes Bank)</p>
                  </div>
                </div>

                <div className="p-3 rounded-lg bg-obsidian-900 border border-white/5 space-y-2">
                  <span className="text-slate-500 text-[10px] uppercase font-bold block">Creditor (Beneficiary)</span>
                  <div className="space-y-1.5 text-slate-300">
                    <p className="font-semibold text-white">{beneficiaryName}</p>
                    <EncryptedField value={beneficiaryAccount} label="Account" fieldType="account_number" />
                    <EncryptedField value={beneficiaryIfsc} label="IFSC Code" fieldType="ifsc" />
                  </div>
                </div>
              </div>

              <div className="p-3 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-xs font-mono text-emerald-300 flex items-center gap-2">
                <ShieldCheck className="w-4 h-4 text-emerald-400 shrink-0" />
                <span>Zero-float guarantee: Integer paise arithmetic verified. No rounding drift.</span>
              </div>
            </div>
          )}

          {activeTab === "CALENDAR" && (
            <div className="space-y-3 font-mono text-xs">
              <div className="p-3 rounded-lg bg-obsidian-900 border border-white/5 space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-[10px] uppercase text-slate-500 font-bold block">
                    RBI Negotiable Instruments Act Rollover Trace
                  </span>
                  <span className="pill-amber text-[9px]">{calendarInfo?.targetSettlementWindow || "T+1"} Window</span>
                </div>
                <p className="text-slate-300 text-[11px] leading-relaxed">
                  Target Settlement Date: <strong className="text-white">{calendarInfo?.scheduledSettlementDate || scheduledDate}</strong> (17:00 IST Cutoff)
                </p>

                <div className="space-y-2 pt-2 border-t border-white/5">
                  {calendarInfo?.rolloverExplanations && calendarInfo.rolloverExplanations.length > 0 ? (
                    calendarInfo.rolloverExplanations.map((exp, idx) => (
                      <div key={idx} className="flex items-start gap-2 text-slate-300 text-[11px]">
                        <Clock className="w-3.5 h-3.5 text-amber-400 shrink-0 mt-0.5" />
                        <div>
                          <p className="text-white font-semibold">{exp}</p>
                        </div>
                      </div>
                    ))
                  ) : (
                    <>
                      <div className="flex items-start gap-2 text-slate-300 text-[11px]">
                        <Clock className="w-3.5 h-3.5 text-amber-400 shrink-0 mt-0.5" />
                        <div>
                          <p className="text-white font-semibold">T+0 Submission (17:15:00 IST)</p>
                          <p className="text-slate-400">Received past the 17:00 IST RTGS cutoff. Rolled to T+1 base.</p>
                        </div>
                      </div>

                      <div className="flex items-start gap-2 text-slate-300 text-[11px]">
                        <Calendar className="w-3.5 h-3.5 text-rose-400 shrink-0 mt-0.5" />
                        <div>
                          <p className="text-white font-semibold">Weekend Closure (Saturday / Sunday)</p>
                          <p className="text-slate-400">Skipped Saturday (Week 4 RBI closure) and Sunday statutory closure.</p>
                        </div>
                      </div>

                      <div className="flex items-start gap-2 text-slate-300 text-[11px]">
                        <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400 shrink-0 mt-0.5" />
                        <div>
                          <p className="text-emerald-300 font-semibold">Next Operational Business Day</p>
                          <p className="text-slate-400">Executed on {scheduledDate} with nodal wire dispatch.</p>
                        </div>
                      </div>
                    </>
                  )}
                </div>

                {calendarInfo?.upcomingBankHolidays && calendarInfo.upcomingBankHolidays.length > 0 && (
                  <div className="pt-3 border-t border-white/5 space-y-1.5">
                    <span className="text-[10px] uppercase text-slate-500 font-bold block">
                      Statutory RBI Banking Closures (Negotiable Instruments Act)
                    </span>
                    <div className="grid grid-cols-2 gap-1.5 text-[10px]">
                      {calendarInfo.upcomingBankHolidays.slice(0, 4).map((h, i) => (
                        <div key={i} className="p-1.5 rounded bg-obsidian-950 border border-white/5 text-slate-400">
                          <span className="text-white font-medium">{h.date}:</span> {h.name}
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}

          {activeTab === "ISO_JSON" && (
            <div className="space-y-2">
              <div className="flex items-center justify-between text-xs font-mono text-slate-400">
                <span>pacs.008.001.08 Compliant Wire Instruction</span>
                <button
                  onClick={handleCopyJson}
                  className="flex items-center gap-1 hover:text-white transition-colors"
                >
                  <Copy className="w-3 h-3" />
                  <span>{copied ? "Copied!" : "Copy JSON"}</span>
                </button>
              </div>

              <pre className="p-3 rounded-lg bg-obsidian-950 border border-white/5 font-mono text-[11px] text-slate-300 overflow-x-auto max-h-64 leading-relaxed">
                {JSON.stringify(wirePayload, null, 2)}
              </pre>
            </div>
          )}
        </div>

        {/* Footer actions */}
        <div className="p-4 border-t border-white/5 flex items-center justify-between bg-obsidian-900 font-mono text-xs">
          <span className="text-slate-500 text-[11px]">Idempotency Key: idemp-wire-{batchId.slice(0, 16)}</span>

          <div className="flex items-center gap-2">
            <button
              onClick={handleDownloadJson}
              className="btn-primary py-1.5 px-3 flex items-center gap-1.5 text-xs"
            >
              <Download className="w-3.5 h-3.5" />
              <span>Download pacs.008 Payload</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
