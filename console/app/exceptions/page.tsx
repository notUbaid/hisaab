"use client";

import React, { useState, useMemo, useEffect } from "react";
import {
  AlertTriangle,
  Filter,
  CheckCircle2,
  Clock,
  ShieldAlert,
  ArrowRight,
  Download,
  RotateCcw,
  Search,
  RefreshCw,
  Lock,
} from "lucide-react";
import MoneyCell from "@/components/MoneyCell";
import { ReconDiffDrawer } from "@/components/ReconDiffDrawer";
import { EncryptedField } from "@/components/EncryptedField";
import { ReconException, SCENARIOS_DATA } from "@/lib/scenarioData";
import { ParticipantPerspective } from "@/lib/rbac";
import { apiClient } from "@/lib/apiClient";

export default function ExceptionsPage() {
  const [selectedException, setSelectedException] = useState<ReconException | null>(null);
  const [filterClass, setFilterClass] = useState<string>("ALL");
  const [filterAgeing, setFilterAgeing] = useState<string>("ALL");
  const [filterStatus, setFilterStatus] = useState<string>("ALL");
  const [searchQuery, setSearchQuery] = useState<string>("");
  const [perspective, setPerspective] = useState<ParticipantPerspective>("COLLECTOR_RSP");
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [sourceBanner, setSourceBanner] = useState<string | null>(null);

  // Aggregate exceptions across all scenarios as baseline
  const [exceptions, setExceptions] = useState<ReconException[]>(() => {
    const list: ReconException[] = [];
    SCENARIOS_DATA.forEach((s) => {
      s.exceptions.forEach((e) => {
        list.push({ ...e });
      });
    });
    return list;
  });

  // Fetch live exceptions from Go backend API via /api/recon
  const fetchExceptions = async () => {
    setIsLoading(true);
    try {
      const res = await apiClient.getExceptions();

      if (res.exceptions && res.exceptions.length > 0) {
        const mapped: ReconException[] = res.exceptions.map((e, idx) => ({
          id: e.id || e.exception_id || `EXC-${idx + 1}`,
          class: e.class || e.exception_class || "AMOUNT_MISMATCH",
          orderId: e.orderId || e.network_order_id || `ORD-${1000 + idx}`,
          expectedPaise: e.expectedPaise || e.expected_paise || 100000,
          actualPaise: e.actualPaise || e.actual_paise || 99000,
          variancePaise: e.variancePaise || e.variance_paise || -1000,
          ageingDays: e.ageingDays || e.ageing_days || 1,
          status: (e.status === "RESOLVED" ? "RESOLVED" : "OPEN") as "OPEN" | "RESOLVED",
          details: e.details || "Reconciliation discrepancy detected",
          remedyAction: e.remedyAction || e.remedy_action || "Pending operator triage",
        }));
        setExceptions(mapped);
        setSourceBanner(res.source === "live_go_engine" ? "Live Go Engine API" : "Scenario Catalog");
      }
    } catch (_err) {
      setSourceBanner("Scenario Catalog (Go Engine Offline)");
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchExceptions();
  }, []);

  // Filtered exceptions
  const filteredExceptions = useMemo(() => {
    return exceptions.filter((e) => {
      const matchesSearch =
        !searchQuery ||
        e.id.toLowerCase().includes(searchQuery.toLowerCase()) ||
        e.orderId.toLowerCase().includes(searchQuery.toLowerCase()) ||
        e.class.toLowerCase().includes(searchQuery.toLowerCase());
      const matchesClass = filterClass === "ALL" || e.class.includes(filterClass);
      const matchesAgeing = filterAgeing === "ALL" || `T+${e.ageingDays}` === filterAgeing;
      const matchesStatus = filterStatus === "ALL" || e.status === filterStatus;
      return matchesSearch && matchesClass && matchesAgeing && matchesStatus;
    });
  }, [exceptions, searchQuery, filterClass, filterAgeing, filterStatus]);

  const handleResolveException = (id: string, remedy: string) => {
    setExceptions((prev) =>
      prev.map((e) =>
        e.id === id ? { ...e, status: "RESOLVED" as const, remedyAction: remedy } : e
      )
    );
    if (selectedException && selectedException.id === id) {
      setSelectedException((prev) => (prev ? { ...prev, status: "RESOLVED" as const } : null));
    }
  };

  const getRootCausePill = (exc: ReconException) => {
    const cls = exc.class.toLowerCase();
    if (cls.includes("tax") || cls.includes("tcs") || cls.includes("rounding")) {
      return {
        label: "TCS Rounding Drift (₹0.01)",
        clsName: "pill-amber",
      };
    }
    if (cls.includes("unmatched") || cls.includes("missing")) {
      return {
        label: "Missing Counterpart Claim",
        clsName: "pill-rose",
      };
    }
    if (cls.includes("fee") || cls.includes("bff") || cls.includes("commission")) {
      return {
        label: "BFF Component Mismatch",
        clsName: "pill-amber",
      };
    }
    return {
      label: "Late Settlement Carrier",
      clsName: "pill-slate",
    };
  };

  const openCount = exceptions.filter((e) => e.status !== "RESOLVED").length;
  const resolvedCount = exceptions.filter((e) => e.status === "RESOLVED").length;

  return (
    <div className="flex-1 overflow-y-auto p-6 space-y-6 max-w-7xl mx-auto w-full font-sans">
      {/* ── Top Header Strip ── */}
      <div className="flex flex-wrap items-center justify-between gap-4 pb-2 border-b border-white/5">
        <div>
          <h1 className="text-xl font-bold text-white tracking-tight flex items-center gap-2.5">
            Reconciliation Exceptions Triage
            <span className="pill-rose text-[10px]">{openCount} Active</span>
            {sourceBanner && (
              <span className="px-2 py-0.5 rounded bg-white/5 border border-white/10 text-slate-400 font-mono text-[10px]">
                {sourceBanner}
              </span>
            )}
          </h1>
          <p className="text-xs text-slate-400 font-mono mt-1">
            Automated Discrepancy Diagnostics, 3-Way Payload Diff & Settlement Remediation
          </p>
        </div>

        {/* Quick Summary Counts & Refresh */}
        <div className="flex items-center gap-2 font-mono text-xs">
          <button
            onClick={fetchExceptions}
            disabled={isLoading}
            className="p-1.5 rounded-lg bg-obsidian-850 border border-white/10 text-slate-300 hover:text-white hover:bg-white/5 transition-colors"
            title="Refresh Exceptions"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? "animate-spin text-emerald-400" : ""}`} />
          </button>
          <div className="px-3 py-1.5 rounded-lg bg-obsidian-850 border border-white/10 text-slate-300">
            Total: <strong className="text-white">{exceptions.length}</strong>
          </div>
          <div className="px-3 py-1.5 rounded-lg bg-amber-500/10 border border-amber-500/30 text-amber-300">
            Open: <strong className="text-amber-400">{openCount}</strong>
          </div>
          <div className="px-3 py-1.5 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-emerald-300">
            Resolved: <strong className="text-emerald-400">{resolvedCount}</strong>
          </div>
        </div>
      </div>

      {/* ── Filters & Search Bar ── */}
      <div className="p-3.5 rounded-lg bg-obsidian-850 border border-white/5 flex flex-wrap items-center justify-between gap-3 text-xs font-mono">
        <div className="flex items-center gap-2 flex-1 max-w-md">
          <Search className="w-4 h-4 text-slate-500" />
          <input
            type="text"
            placeholder="Search by exception ID, order reference, or class..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full bg-obsidian-900 border border-white/10 rounded-md px-2.5 py-1.5 text-xs text-white placeholder:text-slate-500 focus:outline-none focus:border-emerald-500/50"
          />
        </div>

        <div className="flex items-center gap-3">
          {/* Class Filter */}
          <div className="flex items-center gap-1.5">
            <span className="text-slate-500 text-[10px] uppercase">Class:</span>
            <select
              value={filterClass}
              onChange={(e) => setFilterClass(e.target.value)}
              className="bg-obsidian-900 border border-white/10 text-slate-300 px-2 py-1 rounded text-xs focus:outline-none"
            >
              <option value="ALL">All Classes</option>
              <option value="AMOUNT">Amount Mismatch</option>
              <option value="TAX">Tax Withholding</option>
              <option value="TIMING">Timing Delay</option>
            </select>
          </div>

          {/* Ageing Filter */}
          <div className="flex items-center gap-1.5">
            <span className="text-slate-500 text-[10px] uppercase">Ageing:</span>
            <select
              value={filterAgeing}
              onChange={(e) => setFilterAgeing(e.target.value)}
              className="bg-obsidian-900 border border-white/10 text-slate-300 px-2 py-1 rounded text-xs focus:outline-none"
            >
              <option value="ALL">All Ageing</option>
              <option value="T+0">T+0 (Same-day)</option>
              <option value="T+1">T+1 (Standard)</option>
              <option value="T+2">T+2 (Escalation)</option>
            </select>
          </div>

          {/* Status Filter */}
          <div className="flex items-center gap-1.5">
            <span className="text-slate-500 text-[10px] uppercase">Status:</span>
            <select
              value={filterStatus}
              onChange={(e) => setFilterStatus(e.target.value)}
              className="bg-obsidian-900 border border-white/10 text-slate-300 px-2 py-1 rounded text-xs focus:outline-none"
            >
              <option value="ALL">All Status</option>
              <option value="OPEN">Open</option>
              <option value="RESOLVED">Resolved</option>
            </select>
          </div>
        </div>
      </div>

      {/* ── Exceptions Table ── */}
      <div className="fin-card overflow-hidden">
        <table className="w-full text-left border-collapse">
          <thead>
            <tr>
              <th className="fin-table-th">Exc ID</th>
              <th className="fin-table-th">Order Ref</th>
              <th className="fin-table-th">Beneficiary Account</th>
              <th className="fin-table-th">Root Cause Pill</th>
              <th className="fin-table-th text-right">Expected (₹)</th>
              <th className="fin-table-th text-right">Actual (₹)</th>
              <th className="fin-table-th text-right">Variance</th>
              <th className="fin-table-th">Ageing</th>
              <th className="fin-table-th">Status</th>
              <th className="fin-table-th text-center">Inspect & Action</th>
            </tr>
          </thead>
          <tbody>
            {filteredExceptions.map((exc) => {
              const pill = getRootCausePill(exc);
              const isSelected = selectedException?.id === exc.id;
              return (
                <tr
                  key={exc.id}
                  onClick={() => setSelectedException(exc)}
                  className={`cursor-pointer transition-colors ${
                    isSelected
                      ? "bg-emerald-500/10 border-l-2 border-emerald-500"
                      : "hover:bg-white/[0.02]"
                  }`}
                >
                  <td className="fin-table-td font-semibold text-white">{exc.id}</td>
                  <td className="fin-table-td text-slate-400 font-mono text-xs">{exc.orderId}</td>
                  <td className="fin-table-td">
                    <EncryptedField
                      value="918237461928"
                      label="Beneficiary Account"
                      fieldType="account_number"
                      className="text-[11px]"
                    />
                  </td>
                  <td className="fin-table-td">
                    <span className={pill.clsName}>{pill.label}</span>
                  </td>
                  <td className="fin-table-td text-right font-medium text-slate-200">
                    <MoneyCell paise={exc.expectedPaise} />
                  </td>
                  <td className="fin-table-td text-right font-medium text-slate-200">
                    <MoneyCell paise={exc.actualPaise} />
                  </td>
                  <td className="fin-table-td text-right font-bold text-rose-400">
                    <MoneyCell paise={exc.variancePaise} direction="net" showSign />
                  </td>
                  <td className="fin-table-td text-slate-400 font-mono">
                    <span className="px-1.5 py-0.5 rounded bg-white/5 border border-white/5 text-[10px]">
                      T+{exc.ageingDays}
                    </span>
                  </td>
                  <td className="fin-table-td">
                    <span
                      className={
                        exc.status === "RESOLVED" ? "pill-emerald" : "pill-amber"
                      }
                    >
                      {exc.status}
                    </span>
                  </td>
                  <td className="fin-table-td text-center">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        setSelectedException(exc);
                      }}
                      className="btn-ghost text-[10px] py-0.5 px-2.5 inline-flex items-center gap-1"
                    >
                      <span>Inspect Diff</span>
                      <ArrowRight className="w-3 h-3" />
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>

        {filteredExceptions.length === 0 && (
          <div className="p-12 text-center text-slate-500 font-mono text-xs">
            No reconciliation exceptions found matching the current filters.
          </div>
        )}
      </div>

      {/* ── Slide-Out Recon Diff Drawer ── */}
      <ReconDiffDrawer
        exception={selectedException}
        onClose={() => setSelectedException(null)}
        onResolve={handleResolveException}
        perspective={perspective}
      />
    </div>
  );
}
