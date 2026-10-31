"use client";

import React, { useState, useEffect } from "react";
import {
  Activity,
  Database,
  CheckCircle2,
  AlertCircle,
  Clock,
  Layers,
  RefreshCw,
  Server,
  Zap,
} from "lucide-react";
import { apiClient } from "@/lib/apiClient";

interface WorkerTelemetryData {
  source?: string;
  uptime_seconds?: number;
  database_connected?: boolean;
  pending_outbox_events?: number;
  dispatched_outbox_events?: number;
  failed_outbox_events?: number;
  total_batches_cached?: number;
  timestamp?: string;
  warning?: string;
}

export function WorkerMonitor({ className = "" }: { className?: string }) {
  const [data, setData] = useState<WorkerTelemetryData | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [lastRefreshed, setLastRefreshed] = useState<Date>(new Date());

  const fetchTelemetry = async () => {
    setIsLoading(true);
    try {
      const res = await apiClient.get<WorkerTelemetryData>("/api/worker/telemetry");
      setData(res);
      setLastRefreshed(new Date());
    } catch (_e) {
      setData({
        source: "offline_fallback",
        uptime_seconds: 0,
        database_connected: false,
        pending_outbox_events: 0,
        dispatched_outbox_events: 0,
        failed_outbox_events: 0,
        total_batches_cached: 0,
        warning: "Telemetry unreachable",
      });
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchTelemetry();
    const interval = setInterval(fetchTelemetry, 15000); // 15s polling
    return () => clearInterval(interval);
  }, []);

  const formatUptime = (seconds?: number) => {
    if (!seconds || seconds <= 0) return "0m";
    const hrs = Math.floor(seconds / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    if (hrs > 0) return `${hrs}h ${mins}m`;
    return `${mins}m`;
  };

  const isLive = data?.source === "live_go_worker";

  return (
    <div
      className={`rounded-xl border border-white/10 bg-obsidian-850 p-4 transition-all duration-200 hover:border-white/15 ${className}`}
    >
      <div className="flex items-center justify-between pb-3 border-b border-white/5">
        <div className="flex items-center gap-2">
          <div className="relative">
            <Activity className="w-4 h-4 text-emerald-400" />
            <span className="absolute -top-0.5 -right-0.5 w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
          </div>
          <span className="font-sans text-xs font-semibold uppercase tracking-wider text-slate-200">
            Worker Engine & Outbox Rail
          </span>
          <span
            className={`px-1.5 py-0.5 rounded text-[10px] font-mono uppercase tracking-tight ${
              isLive
                ? "bg-emerald-500/10 text-emerald-400 border border-emerald-500/30"
                : "bg-amber-500/10 text-amber-400 border border-amber-500/30"
            }`}
          >
            {isLive ? "LIVE ENGINE" : "SIMULATOR"}
          </span>
        </div>

        <div className="flex items-center gap-2">
          <span className="text-[10px] text-slate-500 font-mono">
            {lastRefreshed.toLocaleTimeString([], { hour12: false })}
          </span>
          <button
            onClick={fetchTelemetry}
            disabled={isLoading}
            className="p-1 rounded text-slate-400 hover:text-white hover:bg-white/5 transition-colors"
            title="Refresh Worker Telemetry"
          >
            <RefreshCw className={`w-3 h-3 ${isLoading ? "animate-spin text-emerald-400" : ""}`} />
          </button>
        </div>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 pt-3 font-mono">
        {/* Outbox Queue Pending */}
        <div className="rounded-lg bg-obsidian-900 border border-white/5 p-2.5">
          <div className="text-[10px] text-slate-400 flex items-center gap-1 mb-1">
            <Clock className="w-3 h-3 text-amber-400" />
            <span>Outbox Pending</span>
          </div>
          <div className="text-sm font-bold text-amber-300 tabular-nums">
            {data?.pending_outbox_events ?? 0}
          </div>
          <div className="text-[9px] text-slate-500 mt-0.5">Wire dispatch queue</div>
        </div>

        {/* Outbox Dispatched */}
        <div className="rounded-lg bg-obsidian-900 border border-white/5 p-2.5">
          <div className="text-[10px] text-slate-400 flex items-center gap-1 mb-1">
            <CheckCircle2 className="w-3 h-3 text-emerald-400" />
            <span>Dispatched Wires</span>
          </div>
          <div className="text-sm font-bold text-emerald-400 tabular-nums">
            {data?.dispatched_outbox_events ?? 0}
          </div>
          <div className="text-[9px] text-slate-500 mt-0.5">ISO 20022 pacs.008</div>
        </div>

        {/* Database Persistence Engine */}
        <div className="rounded-lg bg-obsidian-900 border border-white/5 p-2.5">
          <div className="text-[10px] text-slate-400 flex items-center gap-1 mb-1">
            <Database className="w-3 h-3 text-blue-400" />
            <span>Storage Spine</span>
          </div>
          <div className="text-xs font-semibold text-white truncate">
            {data?.database_connected ? "PostgreSQL 16" : "In-Memory MemLedger"}
          </div>
          <div className="text-[9px] text-slate-500 mt-0.5">
            {data?.database_connected ? "Transactional DDL" : "Single-node fallback"}
          </div>
        </div>

        {/* Engine Uptime */}
        <div className="rounded-lg bg-obsidian-900 border border-white/5 p-2.5">
          <div className="text-[10px] text-slate-400 flex items-center gap-1 mb-1">
            <Server className="w-3 h-3 text-purple-400" />
            <span>Uptime</span>
          </div>
          <div className="text-sm font-bold text-purple-300 tabular-nums">
            {formatUptime(data?.uptime_seconds)}
          </div>
          <div className="text-[9px] text-slate-500 mt-0.5">
            {data?.total_batches_cached ?? 0} active batches
          </div>
        </div>
      </div>
    </div>
  );
}
