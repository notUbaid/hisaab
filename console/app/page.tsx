"use client";

import React, { useState, useEffect } from "react";
import Link from "next/link";
import {
  TrendingUp,
  AlertTriangle,
  CheckCircle2,
  Clock,
  ArrowRight,
  ShieldCheck,
  Calendar,
  Layers,
  ArrowUpRight,
  Play,
  RotateCw,
} from "lucide-react";
import MoneyCell, { formatIndianNumbering } from "@/components/MoneyCell";
import { SCENARIOS_DATA } from "@/lib/scenarioData";
import { WireBatchModal } from "@/components/WireBatchModal";
import { WorkerMonitor } from "@/components/WorkerMonitor";

export default function ExecutiveOverviewPage() {
  const [timeLeft, setTimeLeft] = useState<{ hours: number; minutes: number; seconds: number }>({
    hours: 4,
    minutes: 32,
    seconds: 18,
  });
  const [isSimulating, setIsSimulating] = useState(false);
  const [simMessage, setSimMessage] = useState<string | null>(null);
  const [isSimError, setIsSimError] = useState(false);
  const [isWireModalOpen, setIsWireModalOpen] = useState(false);

  // Live countdown to 17:00:00 IST cutoff
  useEffect(() => {
    const timer = setInterval(() => {
      setTimeLeft((prev) => {
        if (prev.seconds > 0) return { ...prev, seconds: prev.seconds - 1 };
        if (prev.minutes > 0) return { ...prev, minutes: 59, seconds: 59 };
        if (prev.hours > 0) return { hours: prev.hours - 1, minutes: 59, seconds: 59 };
        return { hours: 23, minutes: 59, seconds: 59 };
      });
    }, 1000);
    return () => clearInterval(timer);
  }, []);

  const handleTriggerBatch = async () => {
    setIsSimulating(true);
    setIsSimError(false);
    setSimMessage("Triggering Live ONDC Settle Batch...");
    try {
      const res = await fetch("/api/scenarios/run", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ scenarioId: "ec-03", targetMode: "live_api" }),
      });
      const data = await res.json();
      setIsSimulating(false);
      if (res.ok && data.passed) {
        setIsSimError(false);
        setSimMessage(`Batch Settled (${data.latencyMs ?? 14}ms)`);
      } else {
        setIsSimError(true);
        setSimMessage(`Batch Failed: ${data.error || data.serverStatus || "Execution failed"}`);
      }
      setTimeout(() => {
        setSimMessage(null);
        setIsSimError(false);
      }, 4000);
    } catch {
      setIsSimulating(false);
      setIsSimError(true);
      setSimMessage("Engine Unavailable: Offline");
      setTimeout(() => {
        setSimMessage(null);
        setIsSimError(false);
      }, 4000);
    }
  };

  // Sparkline SVG helper
  const renderSparkline = (data: number[], color: string) => {
    const min = Math.min(...data);
    const max = Math.max(...data);
    const range = max - min || 1;
    const width = 100;
    const height = 28;

    const points = data
      .map((val, idx) => {
        const x = (idx / (data.length - 1)) * width;
        const y = height - ((val - min) / range) * (height - 4) - 2;
        return `${x},${y}`;
      })
      .join(" ");

    return (
      <svg width={width} height={height} className="overflow-visible">
        <polyline
          fill="none"
          stroke={color}
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          points={points}
        />
      </svg>
    );
  };

  return (
    <div className="flex-1 overflow-y-auto p-6 space-y-6 max-w-7xl mx-auto w-full">
      {/* ── Top Executive Title & Window Banner ── */}
      <div className="flex flex-wrap items-center justify-between gap-4 pb-2 border-b border-white/5">
        <div>
          <h1 className="text-xl font-bold text-white tracking-tight flex items-center gap-2.5">
            Executive Settlement Overview
            <span className="pill-emerald text-[10px]">T+1 Active</span>
          </h1>
          <p className="text-xs text-slate-400 font-mono mt-1">
            ONDC RSF 2.0 Clearing & Multi-Party Nodal Escrow Dashboard
          </p>
        </div>

        <div className="flex items-center gap-3">
          {/* Live Cutoff Countdown Timer */}
          <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-obsidian-850 border border-white/10 font-mono text-xs">
            <Clock className="w-4 h-4 text-amber-400" />
            <span className="text-slate-400">Next RTGS Cutoff (17:00 IST):</span>
            <span className="text-amber-400 font-bold tabular-nums">
              {String(timeLeft.hours).padStart(2, "0")}:{String(timeLeft.minutes).padStart(2, "0")}:
              {String(timeLeft.seconds).padStart(2, "0")}
            </span>
          </div>

          {/* Trigger Batch Run */}
          <button
            onClick={handleTriggerBatch}
            disabled={isSimulating}
            className={`flex items-center gap-2 py-2 px-3.5 text-xs transition-colors rounded-lg font-medium ${
              isSimError
                ? "bg-rose-500/10 text-rose-300 border border-rose-500/40"
                : "btn-primary shadow-glow"
            }`}
          >
            {isSimulating ? (
              <>
                <RotateCw className="w-3.5 h-3.5 animate-spin" />
                <span>Running...</span>
              </>
            ) : isSimError ? (
              <>
                <AlertTriangle className="w-3.5 h-3.5 text-rose-400" />
                <span>{simMessage || "Trigger Batch Settle"}</span>
              </>
            ) : (
              <>
                <Play className="w-3.5 h-3.5 fill-current" />
                <span>{simMessage || "Trigger Batch Settle"}</span>
              </>
            )}
          </button>
        </div>
      </div>

      {/* ── Statutory RBI Holiday Banner ── */}
      <div
        onClick={() => setIsWireModalOpen(true)}
        className="p-3 rounded-lg bg-obsidian-850 border border-amber-500/20 flex items-center justify-between text-xs font-mono cursor-pointer hover:bg-obsidian-800 transition-colors"
      >
        <div className="flex items-center gap-2.5">
          <Calendar className="w-4 h-4 text-amber-400 shrink-0" />
          <span className="text-slate-300">
            RBI Statutory Settlement Calendar:{" "}
            <strong className="text-white font-medium">Upcoming Holiday — Diwali (Balipratipada)</strong>
          </span>
          <span className="pill-amber text-[9px]">Rolls to Next Business Day</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-[11px] text-slate-400">Reference: RBI Section 25 NI Act</span>
          <span className="text-[10px] text-amber-400 underline">Inspect Rollover Logic →</span>
        </div>
      </div>

      {/* ── 1. Four-Pillar KPI Ribbon ── */}
      <div className="grid grid-cols-4 gap-4">
        {/* Pillar 1: Gross Settlement Volume */}
        <div className="fin-card p-4 space-y-3">
          <div className="flex justify-between items-start">
            <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">
              Gross Volume
            </span>
            <span className="pill-emerald text-[9px]">+8.4% 7d</span>
          </div>
          <div>
            <div className="text-xl font-bold font-mono text-white tracking-tight">
              <MoneyCell paise={4895000000} withPopover={false} />
            </div>
            <div className="text-[10px] text-slate-500 font-mono mt-0.5">
              Across 4,210 network claims
            </div>
          </div>
          <div className="pt-2 border-t border-white/5 flex items-center justify-between">
            <span className="text-[10px] text-slate-500 font-mono">7-Day Trend</span>
            {renderSparkline([42, 45, 41, 49, 52, 50, 58], "#10B981")}
          </div>
        </div>

        {/* Pillar 2: Net Settled Value */}
        <div className="fin-card p-4 space-y-3">
          <div className="flex justify-between items-start">
            <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">
              Net Settled Value
            </span>
            <span className="pill-emerald text-[9px]">99.4% Gross</span>
          </div>
          <div>
            <div className="text-xl font-bold font-mono text-emerald-400 tracking-tight">
              <MoneyCell paise={4680000000} withPopover={false} />
            </div>
            <div className="text-[10px] text-slate-500 font-mono mt-0.5">
              Net wire instructions issued
            </div>
          </div>
          <div className="pt-2 border-t border-white/5 flex items-center justify-between">
            <span className="text-[10px] text-slate-500 font-mono">7-Day Trend</span>
            {renderSparkline([40, 43, 39, 47, 50, 48, 56], "#10B981")}
          </div>
        </div>

        {/* Pillar 3: Unreconciled Variance */}
        <div className="fin-card p-4 space-y-3">
          <div className="flex justify-between items-start">
            <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">
              Recon Variance
            </span>
            <span className="pill-amber text-[9px]">0.08 bps</span>
          </div>
          <div>
            <div className="text-xl font-bold font-mono text-amber-400 tracking-tight">
              <MoneyCell paise={42000} withPopover={false} />
            </div>
            <div className="text-[10px] text-slate-500 font-mono mt-0.5">
              Within ₹1.00 penny tolerance
            </div>
          </div>
          <div className="pt-2 border-t border-white/5 flex items-center justify-between">
            <span className="text-[10px] text-slate-500 font-mono">7-Day Trend</span>
            {renderSparkline([12, 10, 15, 8, 7, 5, 4], "#F59E0B")}
          </div>
        </div>

        {/* Pillar 4: Open Exceptions */}
        <div className="fin-card p-4 space-y-3">
          <div className="flex justify-between items-start">
            <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">
              Open Exceptions
            </span>
            <span className="pill-rose text-[9px]">3 Requiring Action</span>
          </div>
          <div>
            <div className="text-xl font-bold font-mono text-white tracking-tight flex items-baseline gap-2">
              <span>3</span>
              <span className="text-xs font-normal text-emerald-400">98.2% Auto-Resolved</span>
            </div>
            <div className="text-[10px] text-slate-500 font-mono mt-0.5">
              Avg age: 0.8 days (Max T+2)
            </div>
          </div>
          <div className="pt-2 border-t border-white/5 flex items-center justify-between">
            <span className="text-[10px] text-slate-500 font-mono">7-Day Trend</span>
            {renderSparkline([8, 6, 7, 5, 4, 3, 3], "#F43F5E")}
          </div>
        </div>
      </div>

      {/* ── Worker Telemetry & Outbox Rail ── */}
      <WorkerMonitor />

      {/* ── 2. Interactive Settlement Pipeline Flow Diagram ── */}
      <div className="fin-card p-5 space-y-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Layers className="w-4 h-4 text-emerald-400" />
            <h2 className="font-semibold text-xs text-white uppercase tracking-wider">
              Multi-Party Settlement Pipeline (internal/settlement/orchestrator.go)
            </h2>
          </div>
          <span className="text-[11px] font-mono text-slate-500">Live Stage Velocity</span>
        </div>

        {/* 5-Stage Pipeline Cards */}
        <div className="grid grid-cols-5 gap-3">
          {[
            {
              step: "01",
              name: "Order Confirmed",
              sub: "Beckn /on_confirm",
              amountPaise: 4895000000,
              orders: "4,210 ord",
              status: "INGESTED",
              color: "border-cyan-500/30 text-cyan-400",
            },
            {
              step: "02",
              name: "Collector Escrow",
              sub: "BAP Nodal Account",
              amountPaise: 4895000000,
              orders: "Constrained >= 0",
              status: "FUNDED",
              color: "border-emerald-500/30 text-emerald-400",
            },
            {
              step: "03",
              name: "Receiver NP",
              sub: "BPP Claim Ingest",
              amountPaise: 4875000000,
              orders: "4,198 ord",
              status: "RECEIVED",
              color: "border-purple-500/30 text-purple-400",
            },
            {
              step: "04",
              name: "Recon Matched",
              sub: "Double-Entry Settle",
              amountPaise: 4874958000,
              orders: "TDS/TCS Deducted",
              status: "CONSERVED",
              color: "border-amber-500/30 text-amber-400",
            },
            {
              step: "05",
              name: "Wire Settled",
              sub: "RTGS Bank Dispatch",
              amountPaise: 4680000000,
              orders: "Merkle Rooted",
              status: "DISPATCHED",
              color: "border-emerald-500/40 text-emerald-400",
            },
          ].map((stage, idx) => (
            <div
              key={stage.step}
              className={`p-3.5 rounded-lg bg-obsidian-900 border ${stage.color} space-y-2 relative group hover:bg-obsidian-850 transition-all`}
            >
              <div className="flex justify-between items-center text-[10px] font-mono">
                <span className="text-slate-500 font-bold">{stage.step}</span>
                <span className="font-semibold px-1.5 py-0.2 rounded bg-white/5 border border-white/10 text-[9px]">
                  {stage.status}
                </span>
              </div>
              <div>
                <div className="font-bold text-xs text-white truncate">{stage.name}</div>
                <div className="text-[10px] text-slate-500 font-mono">{stage.sub}</div>
              </div>
              <div className="pt-2 border-t border-white/5 font-mono text-xs">
                <div className="font-semibold text-slate-200">
                  <MoneyCell paise={stage.amountPaise} withPopover={false} />
                </div>
                <div className="text-[10px] text-slate-500 mt-0.5">{stage.orders}</div>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* ── 3. Quick Actions & Edge Cases Navigation ── */}
      <div className="grid grid-cols-3 gap-4">
        {/* Box 1: Exceptions Triage Quick Link */}
        <Link
          href="/exceptions"
          className="fin-card p-4 space-y-2 group hover:border-amber-500/30 transition-all"
        >
          <div className="flex justify-between items-center">
            <span className="font-semibold text-xs text-amber-400 flex items-center gap-1.5">
              <AlertTriangle className="w-4 h-4" />
              Exceptions Triage Workspace
            </span>
            <ArrowUpRight className="w-4 h-4 text-slate-500 group-hover:text-amber-400 transition-colors" />
          </div>
          <p className="text-xs text-slate-400">
            3 active discrepancies. 3-way side-by-side diff between Buyer NP, Seller NP, and Gateway.
          </p>
        </Link>

        {/* Box 2: Scenarios Studio Quick Link */}
        <Link
          href="/scenarios"
          className="fin-card p-4 space-y-2 group hover:border-emerald-500/30 transition-all"
        >
          <div className="flex justify-between items-center">
            <span className="font-semibold text-xs text-emerald-400 flex items-center gap-1.5">
              <ShieldCheck className="w-4 h-4" />
              Edge-Case Scenarios Studio
            </span>
            <ArrowUpRight className="w-4 h-4 text-slate-500 group-hover:text-emerald-400 transition-colors" />
          </div>
          <p className="text-xs text-slate-400">
            Browse all 22 YAML specs (EC-01 to EC-22). Live step execution and visual T-accounts.
          </p>
        </Link>

        {/* Box 3: NP Toolkit & Chaos Quick Link */}
        <Link
          href="/playground"
          className="fin-card p-4 space-y-2 group hover:border-cyan-500/30 transition-all"
        >
          <div className="flex justify-between items-center">
            <span className="font-semibold text-xs text-cyan-400 flex items-center gap-1.5">
              <Clock className="w-4 h-4" />
              NP & Chaos Playground
            </span>
            <ArrowUpRight className="w-4 h-4 text-slate-500 group-hover:text-cyan-400 transition-colors" />
          </div>
          <p className="text-xs text-slate-400">
            Interactive GST/TDS tax builder, schema validation, and failpoint chaos switches.
          </p>
        </Link>
      </div>

      {/* ISO 20022 Wire Batch Inspector Modal */}
      <WireBatchModal
        isOpen={isWireModalOpen}
        onClose={() => setIsWireModalOpen(false)}
        batchId="BATCH-SETTLE-T1"
        amountPaise={4680000000}
        beneficiaryName="State Bank of India (Consolidated Clearing)"
      />
    </div>
  );
}
