"use client";

import React, { useState } from "react";
import { ArrowLeftRight, CheckCircle2, AlertCircle } from "lucide-react";
import MoneyCell from "@/components/MoneyCell";
import { JournalEntry } from "@/lib/scenarioData";

interface LedgerFlowCardProps {
  entries: JournalEntry[];
  title?: string;
  scenarioId?: string;
}

export function LedgerFlowCard({
  entries,
  title = "Double-Entry T-Account Flow",
  scenarioId,
}: LedgerFlowCardProps) {
  const [hoveredEntryId, setHoveredEntryId] = useState<string | null>(null);

  // Group into Debits and Credits
  const debits = entries.filter((e) => e.direction === "debit");
  const credits = entries.filter((e) => e.direction === "credit");

  const totalDebits = debits.reduce((acc, e) => acc + e.amountPaise, 0);
  const totalCredits = credits.reduce((acc, e) => acc + e.amountPaise, 0);
  const isBalanced = totalDebits === totalCredits && entries.length > 0;
  const delta = totalDebits - totalCredits;

  const getAccountBadge = (account: string) => {
    if (account.startsWith("collector:") || account.startsWith("bank:")) {
      return { label: "ASSET / ESCROW", color: "text-cyan-400 bg-cyan-500/10 border-cyan-500/20" };
    }
    if (account.startsWith("receiver:") || account.startsWith("tax:")) {
      return { label: "LIABILITY / PAYABLE", color: "text-purple-400 bg-purple-500/10 border-purple-500/20" };
    }
    if (account.startsWith("fees:")) {
      return { label: "EXPENSE / COMM", color: "text-amber-400 bg-amber-500/10 border-amber-500/20" };
    }
    return { label: "SETTLEMENT ACC", color: "text-slate-400 bg-slate-500/10 border-slate-500/20" };
  };

  return (
    <div className="fin-card overflow-hidden">
      {/* ── Header ── */}
      <div className="px-4 py-3 bg-obsidian-900/90 border-b border-white/5 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <ArrowLeftRight className="w-4 h-4 text-emerald-400" />
          <span className="font-semibold text-xs text-white uppercase tracking-wider">
            {title}
          </span>
          {scenarioId && (
            <span className="font-mono text-[10px] text-slate-500 px-1.5 py-0.5 rounded bg-white/5">
              {scenarioId}
            </span>
          )}
        </div>

        {/* Balance Status Indicator */}
        <div className="flex items-center gap-2 font-mono text-[11px]">
          {isBalanced ? (
            <span className="pill-emerald">
              <CheckCircle2 className="w-3 h-3" />
              <span>Zero-Sum Balanced (Δ = ₹0.00)</span>
            </span>
          ) : (
            <span className="pill-rose">
              <AlertCircle className="w-3 h-3" />
              <span>Unbalanced (Δ = {delta}p)</span>
            </span>
          )}
        </div>
      </div>

      {/* ── T-Account Visual Columns (Debits vs Credits) ── */}
      <div className="grid grid-cols-2 divide-x divide-white/5 font-mono text-xs">
        {/* Left Column: DEBITS (Dr) */}
        <div className="p-4 space-y-3">
          <div className="flex items-center justify-between pb-2 border-b border-white/10 text-slate-400">
            <span className="font-bold text-[11px] uppercase tracking-wider text-amber-400">
              Debits [Dr]
            </span>
            <span className="text-[10px] text-slate-500">Asset + / Liability -</span>
          </div>

          <div className="space-y-2">
            {debits.length > 0 ? (
              debits.map((entry) => {
                const isHovered = hoveredEntryId === entry.id;
                const badge = getAccountBadge(entry.account);
                return (
                  <div
                    key={entry.id}
                    onMouseEnter={() => setHoveredEntryId(entry.id)}
                    onMouseLeave={() => setHoveredEntryId(null)}
                    className={`p-2.5 rounded-lg border transition-all cursor-pointer ${
                      isHovered
                        ? "bg-amber-500/10 border-amber-500/50 shadow-glow-amber"
                        : "bg-obsidian-900/60 border-white/5 hover:border-white/10"
                    }`}
                  >
                    <div className="flex items-center justify-between mb-1">
                      <span className="font-semibold text-slate-200 truncate pr-2">
                        {entry.account}
                      </span>
                      <MoneyCell paise={entry.amountPaise} direction="debit" showSign />
                    </div>
                    <div className="flex items-center justify-between text-[10px] text-slate-500">
                      <span className={`px-1 py-0.2 rounded border text-[9px] ${badge.color}`}>
                        {badge.label}
                      </span>
                      <span className="truncate max-w-[140px]" title={entry.description}>
                        {entry.orderId || entry.description}
                      </span>
                    </div>
                  </div>
                );
              })
            ) : (
              <div className="text-center py-6 text-slate-600 text-[11px]">
                No debit postings
              </div>
            )}
          </div>

          {/* Subtotal Debits */}
          <div className="pt-3 border-t border-white/10 flex justify-between items-center text-xs font-semibold">
            <span className="text-slate-400">Total Debits:</span>
            <MoneyCell paise={totalDebits} direction="debit" />
          </div>
        </div>

        {/* Right Column: CREDITS (Cr) */}
        <div className="p-4 space-y-3">
          <div className="flex items-center justify-between pb-2 border-b border-white/10 text-slate-400">
            <span className="font-bold text-[11px] uppercase tracking-wider text-emerald-400">
              Credits [Cr]
            </span>
            <span className="text-[10px] text-slate-500">Liability + / Asset -</span>
          </div>

          <div className="space-y-2">
            {credits.length > 0 ? (
              credits.map((entry) => {
                const isHovered = hoveredEntryId === entry.id;
                const badge = getAccountBadge(entry.account);
                return (
                  <div
                    key={entry.id}
                    onMouseEnter={() => setHoveredEntryId(entry.id)}
                    onMouseLeave={() => setHoveredEntryId(null)}
                    className={`p-2.5 rounded-lg border transition-all cursor-pointer ${
                      isHovered
                        ? "bg-emerald-500/10 border-emerald-500/50 shadow-glow"
                        : "bg-obsidian-900/60 border-white/5 hover:border-white/10"
                    }`}
                  >
                    <div className="flex items-center justify-between mb-1">
                      <span className="font-semibold text-slate-200 truncate pr-2">
                        {entry.account}
                      </span>
                      <MoneyCell paise={entry.amountPaise} direction="credit" showSign />
                    </div>
                    <div className="flex items-center justify-between text-[10px] text-slate-500">
                      <span className={`px-1 py-0.2 rounded border text-[9px] ${badge.color}`}>
                        {badge.label}
                      </span>
                      <span className="truncate max-w-[140px]" title={entry.description}>
                        {entry.orderId || entry.description}
                      </span>
                    </div>
                  </div>
                );
              })
            ) : (
              <div className="text-center py-6 text-slate-600 text-[11px]">
                No credit postings
              </div>
            )}
          </div>

          {/* Subtotal Credits */}
          <div className="pt-3 border-t border-white/10 flex justify-between items-center text-xs font-semibold">
            <span className="text-slate-400">Total Credits:</span>
            <MoneyCell paise={totalCredits} direction="credit" />
          </div>
        </div>
      </div>
    </div>
  );
}

export default LedgerFlowCard;
