"use client";

import React, { useState, useMemo } from "react";
import {
  FlaskConical,
  Play,
  RotateCw,
  Search,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  ShieldCheck,
  FileCode,
  Tag,
  ArrowRight,
} from "lucide-react";
import MoneyCell from "@/components/MoneyCell";
import { LedgerFlowCard } from "@/components/LedgerFlowCard";
import { ScenarioStepper } from "@/components/ScenarioStepper";
import { MerkleBadge } from "@/components/MerkleBadge";
import { SCENARIOS_DATA, ScenarioItem } from "@/lib/scenarioData";

type CategoryTab = "ALL" | "TAX" | "REFUND" | "COLLECTOR" | "CRASH";

export default function ScenariosStudioPage() {
  const [activeTab, setActiveTab] = useState<CategoryTab>("ALL");
  const [selectedScenarioId, setSelectedScenarioId] = useState<string>("EC-03");
  const [searchQuery, setSearchQuery] = useState<string>("");
  const [isExecuting, setIsExecuting] = useState<boolean>(false);
  const [executionMessage, setExecutionMessage] = useState<string | null>(null);
  const [isExecutionError, setIsExecutionError] = useState<boolean>(false);

  // Active scenario
  const activeScenario = useMemo(() => {
    return SCENARIOS_DATA.find((s) => s.id === selectedScenarioId) || SCENARIOS_DATA[0];
  }, [selectedScenarioId]);

  // Categorize 22 scenarios into the 4 required thematic tabs
  const categorizedScenarios = useMemo(() => {
    return SCENARIOS_DATA.filter((s) => {
      const idNum = parseInt(s.id.replace("EC-", ""), 10);
      let matchesTab = true;

      if (activeTab === "TAX") {
        matchesTab = [3, 6, 7, 17, 18].includes(idNum);
      } else if (activeTab === "REFUND") {
        matchesTab = [4, 5, 16].includes(idNum);
      } else if (activeTab === "COLLECTOR") {
        matchesTab = [1, 2, 8, 9, 10, 11].includes(idNum);
      } else if (activeTab === "CRASH") {
        matchesTab = [12, 13, 14, 15, 19, 20, 21, 22].includes(idNum);
      }

      const q = searchQuery.toLowerCase();
      const matchesSearch =
        !q ||
        s.id.toLowerCase().includes(q) ||
        s.title.toLowerCase().includes(q) ||
        s.npPair.toLowerCase().includes(q);

      return matchesTab && matchesSearch;
    });
  }, [activeTab, searchQuery]);

  // Execute scenario against Go API or CLI runner
  const handleExecuteScenario = async () => {
    setIsExecuting(true);
    setIsExecutionError(false);
    setExecutionMessage("Executing against Go settlement engine...");
    try {
      const res = await fetch("/api/scenarios/run", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ scenarioId: activeScenario.id, targetMode: "live_api" }),
      });
      const data = await res.json();
      setIsExecuting(false);
      if (res.ok && data.passed) {
        setIsExecutionError(false);
        setExecutionMessage(`Verified: ${data.serverStatus} (${data.latencyMs ?? 12}ms)`);
      } else {
        setIsExecutionError(true);
        setExecutionMessage(`Verification Failed: ${data.error || data.serverStatus || "Conformance check failed"}`);
      }
      setTimeout(() => setExecutionMessage(null), 5000);
    } catch {
      setIsExecuting(false);
      setIsExecutionError(true);
      setExecutionMessage("Engine Unavailable: Offline verification failed");
      setTimeout(() => setExecutionMessage(null), 5000);
    }
  };

  return (
    <div className="flex-1 flex overflow-hidden w-full font-sans">
      {/* ── Left Rail: Filterable Scenario Library (280px) ── */}
      <aside className="w-72 min-w-[280px] max-w-[280px] bg-obsidian-850 border-r border-white/5 flex flex-col justify-between shrink-0 select-none">
        <div>
          {/* Header */}
          <div className="p-3 border-b border-white/5 space-y-2">
            <div className="flex items-center justify-between text-xs">
              <span className="font-semibold text-white uppercase tracking-wider flex items-center gap-1.5">
                <FlaskConical className="w-4 h-4 text-emerald-400" />
                Scenarios (22)
              </span>
              <span className="font-mono text-[10px] text-slate-500">EC-01..22</span>
            </div>

            {/* Search Input */}
            <div className="relative">
              <Search className="w-3.5 h-3.5 text-slate-500 absolute left-2.5 top-2" />
              <input
                type="text"
                placeholder="Filter ID, edge case, NP..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full bg-obsidian-900 border border-white/10 rounded-md pl-8 pr-2.5 py-1 text-xs text-white placeholder:text-slate-500 focus:outline-none focus:border-emerald-500/50 font-mono"
              />
            </div>
          </div>

          {/* Grouped Tabs */}
          <div className="p-2 border-b border-white/5 grid grid-cols-2 gap-1 font-mono text-[10px]">
            <button
              onClick={() => setActiveTab("ALL")}
              className={`px-2 py-1 rounded text-center transition ${
                activeTab === "ALL"
                  ? "bg-emerald-500/10 text-emerald-300 font-bold border border-emerald-500/30"
                  : "text-slate-400 hover:bg-white/5"
              }`}
            >
              All (22)
            </button>
            <button
              onClick={() => setActiveTab("TAX")}
              className={`px-2 py-1 rounded text-center transition ${
                activeTab === "TAX"
                  ? "bg-emerald-500/10 text-emerald-300 font-bold border border-emerald-500/30"
                  : "text-slate-400 hover:bg-white/5"
              }`}
            >
              Tax & Withholding
            </button>
            <button
              onClick={() => setActiveTab("REFUND")}
              className={`px-2 py-1 rounded text-center transition ${
                activeTab === "REFUND"
                  ? "bg-emerald-500/10 text-emerald-300 font-bold border border-emerald-500/30"
                  : "text-slate-400 hover:bg-white/5"
              }`}
            >
              Partial Refunds
            </button>
            <button
              onClick={() => setActiveTab("CRASH")}
              className={`px-2 py-1 rounded text-center transition ${
                activeTab === "CRASH"
                  ? "bg-emerald-500/10 text-emerald-300 font-bold border border-emerald-500/30"
                  : "text-slate-400 hover:bg-white/5"
              }`}
            >
              Crash Recovery
            </button>
          </div>

          {/* Scenario List */}
          <div className="overflow-y-auto max-h-[calc(100vh-14rem)] p-2 space-y-1">
            {categorizedScenarios.map((s) => {
              const isSelected = s.id === activeScenario.id;
              let badgeCls = "pill-slate";
              if (s.status === "SETTLED") badgeCls = "pill-emerald";
              else if (s.status === "UNMATCHED") badgeCls = "pill-amber";
              else if (s.status === "INSUFFICIENT_BALANCE") badgeCls = "pill-rose";

              return (
                <div
                  key={s.id}
                  onClick={() => setSelectedScenarioId(s.id)}
                  className={`p-2.5 rounded-lg cursor-pointer transition border text-left ${
                    isSelected
                      ? "bg-emerald-500/10 border-emerald-500/40 shadow-glow"
                      : "bg-obsidian-900/60 border-white/5 hover:border-white/10"
                  }`}
                >
                  <div className="flex items-center justify-between mb-1">
                    <span className="font-mono font-bold text-xs text-white">{s.id}</span>
                    <span className={badgeCls}>{s.status}</span>
                  </div>
                  <p className="text-[11px] text-slate-400 line-clamp-1 leading-snug">
                    {s.title}
                  </p>
                  <div className="flex items-center justify-between text-[10px] text-slate-500 font-mono mt-1 pt-1 border-t border-white/5">
                    <span>{s.ordersCount} orders</span>
                    <span className="text-slate-300 font-semibold">
                      ₹{(s.grossPaise / 100).toLocaleString("en-IN")}
                    </span>
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        {/* Studio Run Button in Rail */}
        <div className="p-3 border-t border-white/5 bg-obsidian-900/90">
          <button
            onClick={handleExecuteScenario}
            disabled={isExecuting}
            className="w-full btn-primary py-2 text-xs flex items-center justify-center gap-2 shadow-glow"
          >
            {isExecuting ? (
              <>
                <RotateCw className="w-3.5 h-3.5 animate-spin" />
                <span>Executing...</span>
              </>
            ) : (
              <>
                <Play className="w-3.5 h-3.5 fill-current" />
                <span>Run {activeScenario.id}</span>
              </>
            )}
          </button>
        </div>
      </aside>

      {/* ── Main Viewport: Stepper + T-Account + Invariants ── */}
      <main className="flex-1 overflow-y-auto p-6 space-y-6">
        {/* Scenario Header Info */}
        <div className="p-4 rounded-xl bg-obsidian-850 border border-white/10 flex flex-wrap items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-2.5">
              <span className="font-mono font-bold text-base text-white">
                {activeScenario.id}:
              </span>
              <span className="font-semibold text-sm text-slate-200">
                {activeScenario.title}
              </span>
            </div>
            <div className="flex items-center gap-3 text-xs text-slate-400 font-mono mt-1">
              <span>NP Pair: <strong className="text-white font-normal">{activeScenario.npPair}</strong></span>
              <span>•</span>
              <span>Citation: <strong className="text-slate-300 font-normal">{activeScenario.brdSection}</strong></span>
              <span>•</span>
              <span>Spec: <strong className="text-emerald-400 font-normal">spec/scenarios/{activeScenario.id.toLowerCase()}.yaml</strong></span>
            </div>
          </div>

          <div className="flex items-center gap-3">
            <MerkleBadge
              rootHash={activeScenario.merkleAudit.rootHash}
              leavesCount={activeScenario.merkleAudit.leavesCount}
              signatureEd25519={activeScenario.merkleAudit.signatureEd25519}
              batchId={`BATCH-${activeScenario.id}`}
            />
          </div>
        </div>

        {executionMessage && (
          <div
            className={`p-3 rounded-lg border text-xs font-mono flex items-center gap-2 animate-in fade-in ${
              isExecutionError
                ? "bg-rose-500/10 border-rose-500/30 text-rose-300"
                : "bg-emerald-500/10 border-emerald-500/30 text-emerald-300"
            }`}
          >
            {isExecutionError ? (
              <AlertTriangle className="w-4 h-4 text-rose-400 shrink-0" />
            ) : (
              <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" />
            )}
            <span>{executionMessage}</span>
          </div>
        )}

        {/* ── Top Half: Visual Double-Entry T-Accounts ── */}
        <LedgerFlowCard
          entries={activeScenario.journalEntries}
          title={`Double-Entry T-Account Postings for ${activeScenario.id}`}
          scenarioId={activeScenario.id}
        />

        {/* ── Bottom Half: DSL Step Execution Timeline ── */}
        <ScenarioStepper
          scenarioId={activeScenario.id}
          onExecute={handleExecuteScenario}
          isExecuting={isExecuting}
        />
      </main>
    </div>
  );
}
