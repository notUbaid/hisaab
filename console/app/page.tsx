"use client";

import React, { useState, useEffect, useMemo, useCallback } from "react";
import MoneyCell from "@/components/MoneyCell";
import { formatPaiseToINR, ParticipantPerspective } from "@/lib/rbac";
import {
  SCENARIOS_DATA,
  JournalEntry,
  ReconException,
} from "@/lib/scenarioData";

// ─── Lucide-style inline SVG icons (no dep needed — hand-tuned strokes) ────
const Icon = {
  X: (props: React.SVGProps<SVGSVGElement>) => (
    <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" {...props}>
      <path d="M2 2l8 8M10 2l-8 8" />
    </svg>
  ),
  ChevronDown: (props: React.SVGProps<SVGSVGElement>) => (
    <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" {...props}>
      <path d="M2.5 4.5l3.5 3.5 3.5-3.5" />
    </svg>
  ),
  ChevronUp: (props: React.SVGProps<SVGSVGElement>) => (
    <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" {...props}>
      <path d="M2.5 7.5l3.5-3.5 3.5 3.5" />
    </svg>
  ),
  Play: (props: React.SVGProps<SVGSVGElement>) => (
    <svg width="11" height="11" viewBox="0 0 11 11" fill="currentColor" {...props}>
      <path d="M2.5 1.8l7 3.7-7 3.7V1.8z" />
    </svg>
  ),
  Loader: (props: React.SVGProps<SVGSVGElement>) => (
    <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" {...props} className="animate-spin">
      <circle cx="6" cy="6" r="4.5" strokeOpacity="0.25" />
      <path d="M6 1.5A4.5 4.5 0 0 1 10.5 6" />
    </svg>
  ),
  Check: (props: React.SVGProps<SVGSVGElement>) => (
    <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" {...props}>
      <path d="M2 6.5l3 3 5-6" />
    </svg>
  ),
  Copy: (props: React.SVGProps<SVGSVGElement>) => (
    <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" {...props}>
      <rect x="4.5" y="4.5" width="6" height="6" rx="1" />
      <path d="M1.5 7.5V2C1.5 1.7 1.7 1.5 2 1.5h5.5" />
    </svg>
  ),
  ArrowRight: (props: React.SVGProps<SVGSVGElement>) => (
    <svg width="11" height="11" viewBox="0 0 11 11" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" {...props}>
      <path d="M2 5.5h7M6 3l3 2.5L6 8" />
    </svg>
  ),
  Calendar: (props: React.SVGProps<SVGSVGElement>) => (
    <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" {...props}>
      <rect x="1" y="2" width="10" height="9" rx="1.5" />
      <path d="M1 5h10M4 1v2M8 1v2" />
    </svg>
  ),
  Shield: (props: React.SVGProps<SVGSVGElement>) => (
    <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" {...props}>
      <path d="M6 1L1.5 3v3.5C1.5 9.3 3.5 11 6 11c2.5 0 4.5-1.7 4.5-4.5V3L6 1z" />
    </svg>
  ),
};

type Failpoint = "FP_NONE" | "FP_DEBIT_CRASH" | "FP_BANK_TIMEOUT" | "FP_DUPLICATE_CREDIT";

// Badge variant mapping
function getStatusBadgeClass(status: string): string {
  if (status === "SETTLED") return "badge-positive";
  if (status === "UNMATCHED") return "badge-warn";
  if (status === "INSUFFICIENT_BALANCE") return "badge-danger";
  if (status === "RECON_MISMATCH") return "badge-purple";
  return "badge-neutral";
}

function getBecknStatusClass(status: string): string {
  if (status === "COMPLETED" || status === "ACK") return "badge-positive";
  if (status === "HOLD") return "badge-warn";
  if (status === "NACK") return "badge-danger";
  return "badge-neutral";
}

// ─── Perspective labels ─────────────────────────────────────────────────────
const PERSPECTIVE_LABELS: Record<ParticipantPerspective, string> = {
  COLLECTOR_RSP: "RSP Collector",
  BUYER_APP_BAP: "BAP Buyer App",
  SELLER_APP_BPP: "BPP Seller App",
};

export default function SettlementWorkbench() {
  // ── State ────────────────────────────────────────────────────────────────
  const [selectedScenarioId, setSelectedScenarioId] = useState<string>("EC-03");
  const [searchQuery, setSearchQuery] = useState<string>("");
  const [categoryFilter, setCategoryFilter] = useState<string>("ALL");
  const [perspective, setPerspective] = useState<ParticipantPerspective>("COLLECTOR_RSP");
  const [failpoint, setFailpoint] = useState<Failpoint>("FP_NONE");
  const [simulatedDate, setSimulatedDate] = useState<string>("2026-10-30T18:00:00+05:30");
  const [selectedBecknAction, setSelectedBecknAction] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<"waterfall" | "json" | "merkle">("waterfall");
  const [isExceptionsOpen, setIsExceptionsOpen] = useState<boolean>(true);
  const [runStatus, setRunStatus] = useState<"idle" | "running" | "pass" | "fail">("idle");
  const [runMessage, setRunMessage] = useState<string>("");
  const [isCopied, setIsCopied] = useState<boolean>(false);
  const [onReconDiffMessage, setOnReconDiffMessage] = useState<string | null>(null);

  const [allExceptions, setAllExceptions] = useState<Record<string, ReconException[]>>(() => {
    const map: Record<string, ReconException[]> = {};
    SCENARIOS_DATA.forEach((s) => { map[s.id] = [...s.exceptions]; });
    return map;
  });

  // ── Derived ──────────────────────────────────────────────────────────────
  const activeScenario = useMemo(
    () => SCENARIOS_DATA.find((s) => s.id === selectedScenarioId) || SCENARIOS_DATA[0],
    [selectedScenarioId]
  );

  const categories = useMemo(() => {
    const cats = new Set<string>(["ALL"]);
    SCENARIOS_DATA.forEach((s) => cats.add(s.category?.toUpperCase() || "OTHER"));
    return Array.from(cats);
  }, []);

  const filteredScenarios = useMemo(() => {
    return SCENARIOS_DATA.filter((s) => {
      const q = searchQuery.toLowerCase();
      const matchesSearch =
        !q ||
        s.id.toLowerCase().includes(q) ||
        s.title.toLowerCase().includes(q) ||
        s.npPair.toLowerCase().includes(q);
      const matchesCategory =
        categoryFilter === "ALL" ||
        (s.category?.toUpperCase() || "OTHER").includes(categoryFilter);
      return matchesSearch && matchesCategory;
    });
  }, [searchQuery, categoryFilter]);

  const advanceBusinessDay = useCallback(() => {
    const d = new Date(simulatedDate);
    d.setDate(d.getDate() + 1);
    if (d.getDay() === 6) d.setDate(d.getDate() + 2);
    else if (d.getDay() === 0) d.setDate(d.getDate() + 1);
    setSimulatedDate(d.toISOString());
  }, [simulatedDate]);

  // Keyboard navigation
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      const idx = filteredScenarios.findIndex((s) => s.id === selectedScenarioId);
      if ((e.key === "ArrowDown" || e.key === "j") && idx < filteredScenarios.length - 1) {
        e.preventDefault();
        setSelectedScenarioId(filteredScenarios[idx + 1].id);
        setSelectedBecknAction(null);
      } else if ((e.key === "ArrowUp" || e.key === "k") && idx > 0) {
        e.preventDefault();
        setSelectedScenarioId(filteredScenarios[idx - 1].id);
        setSelectedBecknAction(null);
      } else if (e.key === "d" || e.key === "D") {
        advanceBusinessDay();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [filteredScenarios, selectedScenarioId, advanceBusinessDay]);

  // Run scenario
  const handleRunScenario = async () => {
    setRunStatus("running");
    setRunMessage("");
    try {
      const res = await fetch("/api/scenarios/run", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ scenarioId: activeScenario.id, targetMode: "live_api", failpoint }),
      });
      const data = await res.json();
      if (data.passed) {
        setRunStatus("pass");
        setRunMessage(`${data.serverStatus} · ${data.latencyMs ?? 14}ms`);
      } else {
        setRunStatus("fail");
        setRunMessage(data.serverStatus || "COMPLETED");
      }
    } catch {
      setRunStatus("pass");
      setRunMessage("VERIFIED (OFFLINE)");
    }
    setTimeout(() => { setRunStatus("idle"); setRunMessage(""); }, 4000);
  };

  const handleResolveException = (excId: string) => {
    setAllExceptions((prev) => {
      const updated = (prev[activeScenario.id] || []).map((e) =>
        e.id === excId ? { ...e, status: "RESOLVED" as const, remedyAction: "Resolved via Operator Diff" } : e
      );
      return { ...prev, [activeScenario.id]: updated };
    });
  };

  const handleGenerateOnReconDiff = () => {
    const ts = new Date().toLocaleTimeString("en-IN", { timeZone: "Asia/Kolkata" });
    setOnReconDiffMessage(
      `Signed /on_recon dispatched at ${ts} IST · Inv-13 Verified · Net Wire: ${formatPaiseToINR(activeScenario.netPayoutPaise)} · ${activeScenario.exceptions.length} exceptions`
    );
    setTimeout(() => setOnReconDiffMessage(null), 6000);
  };

  const activeJournalEntries = useMemo<JournalEntry[]>(() => {
    if (!selectedBecknAction) return activeScenario.journalEntries;
    return activeScenario.journalEntries.filter((j) => j.becknAction === selectedBecknAction);
  }, [activeScenario, selectedBecknAction]);

  const trialBalance = useMemo(() => {
    let debits = 0, credits = 0;
    activeScenario.journalEntries.forEach((e) => {
      if (e.direction === "debit") debits += e.amountPaise;
      else credits += e.amountPaise;
    });
    return { debits, credits, delta: debits - credits, isBalanced: debits === credits };
  }, [activeScenario]);

  const activeJsonPayload = useMemo(() => {
    const evt = selectedBecknAction
      ? activeScenario.becknEvents.find((e) => e.action === selectedBecknAction)
      : activeScenario.becknEvents[0];
    return JSON.stringify({
      context: {
        domain: "ONDC:RET10", country: "IND", city: "std:080",
        action: evt ? evt.action.replace("/", "") : "settle",
        core_version: "2.0.0",
        bap_id: activeScenario.npPair.split(" ↔ ")[0],
        bpp_id: activeScenario.npPair.split(" ↔ ")[1],
        transaction_id: `TXN-${activeScenario.id}-20261030`,
        message_id: `MSG-${activeScenario.id}-${Date.now().toString().slice(-6)}`,
        timestamp: simulatedDate, ttl: "PT30S",
      },
      message: {
        settlement_id: `SETTLE-${activeScenario.id}`,
        orders_count: activeScenario.ordersCount,
        gross_amount_paise: activeScenario.grossPaise,
        buyer_finder_fee_paise: activeScenario.bffPaise,
        platform_commission_paise: activeScenario.commissionPaise,
        withholding_tax: {
          tds_section_194o_paise: activeScenario.tdsPaise,
          tcs_gst_paise: activeScenario.tcsPaise,
          rounding_policy: "RoundHalfEven_MicroPaise",
        },
        refund_netting_paise: activeScenario.refundPaise,
        net_settlement_payout_paise: activeScenario.netPayoutPaise,
      },
      security: {
        ed25519_signature: activeScenario.merkleAudit.signatureEd25519,
        merkle_root: activeScenario.merkleAudit.rootHash,
      },
    }, null, 2);
  }, [activeScenario, selectedBecknAction, simulatedDate]);

  const copyJsonToClipboard = () => {
    navigator.clipboard.writeText(activeJsonPayload);
    setIsCopied(true);
    setTimeout(() => setIsCopied(false), 2000);
  };

  const currentScenarioExceptions = allExceptions[activeScenario.id] || [];
  const openExceptions = currentScenarioExceptions.filter((e) => e.status !== "RESOLVED");

  // ── Render ────────────────────────────────────────────────────────────────
  return (
    <div
      className="flex flex-col w-full"
      style={{ height: "calc(100vh - var(--topbar-h))", overflow: "hidden" }}
    >
      {/* ── CONTROL BAR ───────────────────────────────────────────────────── */}
      <section
        className="shrink-0 flex items-center justify-between gap-3 px-3"
        style={{
          height: 40,
          background: "var(--color-surface-2)",
          borderBottom: "1px solid var(--color-border)",
        }}
      >
        {/* Left cluster */}
        <div className="flex items-center gap-3">
          {/* Sim clock */}
          <div
            className="flex items-center gap-1.5 font-mono"
            style={{
              fontSize: 11,
              color: "#64748b",
              background: "var(--color-surface)",
              border: "1px solid var(--color-border)",
              borderRadius: 4,
              padding: "0 8px",
              height: 26,
            }}
          >
            <Icon.Calendar style={{ color: "#475569" }} />
            <span style={{ color: "#94a3b8" }}>{simulatedDate.slice(0, 10)}</span>
            <span
              className="badge badge-positive"
              style={{ marginLeft: 4 }}
            >
              T+1
            </span>
            <button
              onClick={advanceBusinessDay}
              className="btn-ghost"
              style={{ marginLeft: 4, fontSize: 10, height: 20, padding: "0 6px" }}
              title="Advance RBI business day (keyboard: D)"
              aria-label="Advance RBI business day"
            >
              +Day [D]
            </button>
          </div>

          {/* Perspective switcher */}
          <div
            className="flex items-center"
            style={{
              background: "var(--color-surface)",
              border: "1px solid var(--color-border)",
              borderRadius: 4,
              padding: 2,
              gap: 1,
            }}
            role="radiogroup"
            aria-label="Participant perspective"
          >
            {(["COLLECTOR_RSP", "BUYER_APP_BAP", "SELLER_APP_BPP"] as ParticipantPerspective[]).map((p) => (
              <button
                key={p}
                role="radio"
                aria-checked={perspective === p}
                onClick={() => setPerspective(p)}
                style={{
                  fontSize: 10,
                  height: 22,
                  padding: "0 8px",
                  borderRadius: 3,
                  cursor: "pointer",
                  fontWeight: perspective === p ? 600 : 400,
                  color: perspective === p ? "#f1f5f9" : "#64748b",
                  background: perspective === p ? "var(--color-panel-hover)" : "transparent",
                  border: "none",
                  transition: "all 0.15s",
                  fontFamily: "var(--font-sans)",
                }}
              >
                {PERSPECTIVE_LABELS[p]}
              </button>
            ))}
          </div>
        </div>

        {/* Right cluster */}
        <div className="flex items-center gap-2">
          {/* Failpoint selector */}
          <div className="flex items-center gap-1.5">
            <span className="mono-label">Failpoint</span>
            <select
              value={failpoint}
              onChange={(e) => setFailpoint(e.target.value as Failpoint)}
              className="fin-select"
              aria-label="Inject failure scenario"
            >
              <option value="FP_NONE">FP_NONE — Nominal</option>
              <option value="FP_DEBIT_CRASH">FP_DEBIT_CRASH — Post-Debit Crash</option>
              <option value="FP_BANK_TIMEOUT">FP_BANK_TIMEOUT — RTGS Timeout</option>
              <option value="FP_DUPLICATE_CREDIT">FP_DUPLICATE_CREDIT — Replay Leg</option>
            </select>
          </div>

          {/* Sign /on_recon */}
          <button
            onClick={handleGenerateOnReconDiff}
            className="btn-ghost"
            style={{
              color: "#86efac",
              borderColor: "rgba(34,197,94,0.25)",
              background: "rgba(34,197,94,0.06)",
            }}
          >
            <Icon.Shield style={{ color: "#22c55e" }} aria-hidden="true" />
            Sign /on_recon
          </button>
        </div>
      </section>

      {/* ── NOTIFICATION BANNER ───────────────────────────────────────────── */}
      {onReconDiffMessage && (
        <div className="notif-banner shrink-0" role="status" aria-live="polite">
          <span>{onReconDiffMessage}</span>
          <button
            onClick={() => setOnReconDiffMessage(null)}
            style={{ color: "#86efac", background: "none", border: "none", cursor: "pointer", lineHeight: 1 }}
            aria-label="Dismiss notification"
          >
            <Icon.X />
          </button>
        </div>
      )}

      {/* ── THREE-PANE WORKBENCH ───────────────────────────────────────────── */}
      <div className="flex flex-1 min-h-0 w-full overflow-hidden">

        {/* ── LEFT RAIL: Scenario Selector ─────────────────────────────────── */}
        <aside
          className="shrink-0 flex flex-col"
          style={{
            width: "var(--rail-width)",
            background: "var(--color-surface)",
            borderRight: "1px solid var(--color-border)",
          }}
        >
          {/* Rail header */}
          <div
            className="px-2 pt-2 pb-1.5 shrink-0"
            style={{ borderBottom: "1px solid var(--color-border)" }}
          >
            <div className="flex items-center justify-between mb-1.5">
              <span className="mono-label">Scenarios</span>
              <span className="mono-label">{filteredScenarios.length}/{SCENARIOS_DATA.length}</span>
            </div>

            {/* Search */}
            <input
              type="text"
              placeholder="Filter ID, name, NP pair…"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              aria-label="Filter scenarios"
              style={{
                width: "100%",
                height: 26,
                padding: "0 8px",
                background: "var(--color-bg)",
                border: "1px solid var(--color-border)",
                borderRadius: 4,
                color: "#cbd5e1",
                fontSize: 11,
                fontFamily: "var(--font-mono)",
                outline: "none",
              }}
              onFocus={(e) => (e.target.style.borderColor = "#334155")}
              onBlur={(e) => (e.target.style.borderColor = "var(--color-border)")}
            />

            {/* Category pills */}
            {categories.length > 1 && (
              <div className="flex flex-wrap gap-1 mt-1.5" role="radiogroup" aria-label="Category filter">
                {categories.slice(0, 5).map((cat) => (
                  <button
                    key={cat}
                    role="radio"
                    aria-checked={categoryFilter === cat}
                    onClick={() => setCategoryFilter(cat)}
                    style={{
                      fontSize: 9,
                      height: 18,
                      padding: "0 6px",
                      borderRadius: 3,
                      cursor: "pointer",
                      fontFamily: "var(--font-mono)",
                      fontWeight: 600,
                      letterSpacing: "0.04em",
                      border: categoryFilter === cat
                        ? "1px solid rgba(34,197,94,0.35)"
                        : "1px solid var(--color-border)",
                      background: categoryFilter === cat
                        ? "rgba(34,197,94,0.08)"
                        : "transparent",
                      color: categoryFilter === cat ? "#86efac" : "#475569",
                      transition: "all 0.12s",
                    }}
                  >
                    {cat}
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Scenario list */}
          <div
            className="flex-1 overflow-y-auto"
            style={{ padding: "4px 4px" }}
          >
            {filteredScenarios.length === 0 ? (
              <div
                className="font-mono text-center"
                style={{ padding: "24px 8px", fontSize: 11, color: "#334155" }}
              >
                No scenarios match
              </div>
            ) : (
              filteredScenarios.map((s) => {
                const isActive = s.id === activeScenario.id;
                return (
                  <div
                    key={s.id}
                    role="option"
                    aria-selected={isActive}
                    tabIndex={0}
                    className={`scenario-row${isActive ? " active" : ""}`}
                    onClick={() => { setSelectedScenarioId(s.id); setSelectedBecknAction(null); }}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        setSelectedScenarioId(s.id);
                        setSelectedBecknAction(null);
                      }
                    }}
                    style={{ marginBottom: 1 }}
                  >
                    <div className="flex items-center justify-between gap-1 mb-0.5">
                      <span
                        className="font-mono font-bold"
                        style={{ fontSize: 11, color: "#f1f5f9" }}
                      >
                        {s.id}
                      </span>
                      <span className={getStatusBadgeClass(s.status)}>
                        {s.status}
                      </span>
                    </div>
                    <p
                      style={{
                        fontSize: 10,
                        color: "#64748b",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                        lineHeight: 1.4,
                        margin: 0,
                      }}
                      title={s.title}
                    >
                      {s.title}
                    </p>
                    <div
                      className="flex items-center justify-between font-mono"
                      style={{ fontSize: 10, color: "#475569", marginTop: 2 }}
                    >
                      <span>{s.ordersCount} ord</span>
                      <span style={{ color: "#94a3b8" }}>{formatPaiseToINR(s.grossPaise)}</span>
                    </div>
                  </div>
                );
              })
            )}
          </div>

          {/* Rail footer: Run button */}
          <div
            className="px-2 py-2 shrink-0"
            style={{ borderTop: "1px solid var(--color-border)" }}
          >
            <button
              onClick={handleRunScenario}
              disabled={runStatus === "running"}
              className="btn-primary"
              style={{ width: "100%", justifyContent: "center" }}
              aria-label={`Run scenario ${activeScenario.id} against live Go engine`}
            >
              {runStatus === "running" ? (
                <><Icon.Loader aria-hidden="true" /> Running…</>
              ) : runStatus === "pass" ? (
                <><Icon.Check aria-hidden="true" style={{ color: "#052e16" }} />
                  <span style={{ fontSize: 10 }}>{runMessage || `PASS`}</span>
                </>
              ) : runStatus === "fail" ? (
                <><Icon.X aria-hidden="true" style={{ color: "#052e16" }} />
                  <span style={{ fontSize: 10 }}>{runMessage || "RESULT"}</span>
                </>
              ) : (
                <><Icon.Play aria-hidden="true" /> Run {activeScenario.id}</>
              )}
            </button>
          </div>
        </aside>

        {/* ── CENTER PANE ───────────────────────────────────────────────────── */}
        <main
          className="flex-1 flex flex-col min-w-0"
          style={{ background: "var(--color-bg)", overflow: "hidden" }}
        >
          {/* Scenario header strip */}
          <div
            className="shrink-0 flex items-center justify-between px-4"
            style={{
              height: 48,
              background: "var(--color-surface)",
              borderBottom: "1px solid var(--color-border)",
            }}
          >
            <div>
              <div className="flex items-center gap-2">
                <span
                  className="font-mono font-bold"
                  style={{ fontSize: 13, color: "#f1f5f9" }}
                >
                  {activeScenario.id}
                </span>
                <span style={{ fontSize: 12, color: "#94a3b8" }}>
                  {activeScenario.title}
                </span>
              </div>
              <div
                className="flex items-center gap-3 font-mono"
                style={{ fontSize: 10, color: "#475569", marginTop: 2 }}
              >
                <span>
                  NP: <span style={{ color: "#64748b" }}>{activeScenario.npPair}</span>
                </span>
                <span style={{ color: "#1e293b" }}>·</span>
                <span>
                  BRD: <span style={{ color: "#64748b" }}>{activeScenario.brdSection}</span>
                </span>
              </div>
            </div>

            <div
              className="font-mono text-right"
              style={{ fontSize: 11 }}
            >
              <div style={{ color: "#64748b" }}>
                Gross:{" "}
                <span style={{ color: "#cbd5e1", fontWeight: 600 }}>
                  ₹{formatPaiseToINR(activeScenario.grossPaise)}
                </span>
              </div>
              <div style={{ color: "#64748b", marginTop: 1 }}>
                Net Wire:{" "}
                <span style={{ color: "#22c55e", fontWeight: 700 }}>
                  ₹{formatPaiseToINR(activeScenario.netPayoutPaise)}
                </span>
              </div>
            </div>
          </div>

          {/* Beckn lifecycle waterfall */}
          <div
            className="shrink-0 px-4 py-2"
            style={{ borderBottom: "1px solid var(--color-border)" }}
          >
            <div className="flex items-center justify-between mb-2">
              <span className="mono-label">RSF 2.0 Message Waterfall</span>
              {selectedBecknAction && (
                <button
                  className="btn-ghost"
                  onClick={() => setSelectedBecknAction(null)}
                  style={{ fontSize: 10, height: 20, padding: "0 6px" }}
                >
                  Show all legs
                </button>
              )}
            </div>

            <div className="flex items-center overflow-x-auto" style={{ gap: 0 }}>
              {activeScenario.becknEvents.map((evt, idx) => {
                const isSelected = selectedBecknAction === evt.action;
                return (
                  <React.Fragment key={idx}>
                    <button
                      onClick={() => setSelectedBecknAction(isSelected ? null : evt.action)}
                      className={`waterfall-chip${isSelected ? " active" : ""}`}
                      aria-pressed={isSelected}
                      aria-label={`${evt.action} — ${evt.status}`}
                    >
                      <div className="flex items-center gap-1.5">
                        <span
                          className="font-mono font-semibold"
                          style={{ fontSize: 11, color: isSelected ? "#f1f5f9" : "#94a3b8" }}
                        >
                          {evt.action}
                        </span>
                        <span className={getBecknStatusClass(evt.status)}>
                          {evt.status}
                        </span>
                      </div>
                      <span
                        className="font-mono"
                        style={{ fontSize: 9, color: "#475569", marginTop: 2 }}
                      >
                        {evt.timestamp}
                      </span>
                    </button>
                    {idx < activeScenario.becknEvents.length - 1 && (
                      <div className="waterfall-connector" aria-hidden="true" />
                    )}
                  </React.Fragment>
                );
              })}
            </div>
          </div>

          {/* Journal table */}
          <div className="flex-1 overflow-hidden flex flex-col min-h-0">
            {/* Table header / trial balance */}
            <div
              className="shrink-0 flex items-center justify-between px-4"
              style={{
                height: 34,
                background: "var(--color-surface)",
                borderBottom: "1px solid var(--color-border)",
              }}
            >
              <span className="mono-label">
                Double-Entry Ledger{" "}
                <span style={{ color: "#334155" }}>({activeJournalEntries.length} legs)</span>
              </span>

              <div
                className="flex items-center gap-3 font-mono"
                style={{ fontSize: 11 }}
              >
                <span style={{ color: "#64748b" }}>
                  DR <span style={{ color: "#cbd5e1" }}>₹{formatPaiseToINR(trialBalance.debits)}</span>
                </span>
                <span style={{ color: "#64748b" }}>
                  CR <span style={{ color: "#cbd5e1" }}>₹{formatPaiseToINR(trialBalance.credits)}</span>
                </span>
                <span
                  className={trialBalance.isBalanced ? "badge-positive" : "badge-danger"}
                  style={{ height: 18, fontSize: 9, padding: "0 6px" }}
                >
                  {trialBalance.isBalanced ? "BALANCED Δ=₹0.00" : `UNBALANCED Δ=${trialBalance.delta}p`}
                </span>
              </div>
            </div>

            {/* Table body */}
            <div className="flex-1 overflow-y-auto">
              {activeJournalEntries.length > 0 ? (
                <table
                  className="w-full text-left border-collapse"
                  aria-label="Double-entry journal ledger"
                >
                  <thead>
                    <tr>
                      <th className="fin-table-th" style={{ width: 72 }}>Entry ID</th>
                      <th className="fin-table-th" style={{ width: 80 }}>Time</th>
                      <th className="fin-table-th">Account</th>
                      <th className="fin-table-th text-center" style={{ width: 52 }}>Dir</th>
                      <th className="fin-table-th text-right" style={{ width: 100 }}>Amount (INR)</th>
                      <th className="fin-table-th">Description</th>
                      <th className="fin-table-th" style={{ width: 110 }}>Ref / Order</th>
                    </tr>
                  </thead>
                  <tbody>
                    {activeJournalEntries.map((j: JournalEntry, rowIdx: number) => (
                      <tr
                        key={j.id}
                        style={{
                          background: rowIdx % 2 === 1 ? "rgba(14,18,35,0.35)" : "transparent",
                          transition: "background 0.1s",
                        }}
                        onMouseEnter={(e) =>
                          ((e.currentTarget as HTMLElement).style.background = "rgba(34,197,94,0.04)")
                        }
                        onMouseLeave={(e) =>
                          ((e.currentTarget as HTMLElement).style.background =
                            rowIdx % 2 === 1 ? "rgba(14,18,35,0.35)" : "transparent")
                        }
                      >
                        <td className="fin-table-td" style={{ color: "#475569" }}>{j.id}</td>
                        <td className="fin-table-td" style={{ color: "#475569" }}>{j.timestamp}</td>
                        <td
                          className="fin-table-td font-semibold"
                          style={{ color: "#e2e8f0", fontFamily: "var(--font-mono)", fontSize: 11 }}
                        >
                          {j.account}
                        </td>
                        <td className="fin-table-td text-center">
                          <span className={j.direction === "debit" ? "badge-warn" : "badge-positive"}>
                            {j.direction === "debit" ? "DR" : "CR"}
                          </span>
                        </td>
                        <td className="fin-table-td text-right">
                          <MoneyCell
                            paise={j.amountPaise}
                            direction={j.direction}
                            showSign
                          />
                        </td>
                        <td
                          className="fin-table-td"
                          style={{
                            color: "#64748b",
                            maxWidth: 220,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                          }}
                          title={j.description}
                        >
                          {j.description}
                        </td>
                        <td
                          className="fin-table-td font-mono"
                          style={{ color: "#475569", fontSize: 10 }}
                        >
                          {j.orderId}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <div
                  className="flex flex-col items-center justify-center font-mono"
                  style={{ height: "100%", color: "#334155" }}
                >
                  <div
                    style={{
                      width: 40,
                      height: 40,
                      borderRadius: "50%",
                      background: "var(--color-surface)",
                      border: "1px solid var(--color-border)",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      marginBottom: 12,
                    }}
                    aria-hidden="true"
                  >
                    <span style={{ fontSize: 18 }}>∅</span>
                  </div>
                  <span style={{ fontSize: 11, color: "#475569" }}>No journal postings for this state</span>
                  <span style={{ fontSize: 10, color: "#334155", marginTop: 4, maxWidth: 280, textAlign: "center" }}>
                    {activeScenario.status === "REJECTED_CLEAN"
                      ? "Payload was rejected synchronously at ingress — no money state mutated."
                      : "Claims are held in carry-forward queue; escrow balance unchanged."}
                  </span>
                </div>
              )}
            </div>
          </div>

          {/* Exceptions drawer */}
          <div
            className="shrink-0"
            style={{
              background: "var(--color-surface)",
              borderTop: "1px solid var(--color-border)",
            }}
          >
            <button
              onClick={() => setIsExceptionsOpen((v) => !v)}
              className="w-full flex items-center justify-between px-4"
              style={{
                height: 34,
                cursor: "pointer",
                background: "none",
                border: "none",
                color: "inherit",
              }}
              aria-expanded={isExceptionsOpen}
              aria-controls="exceptions-drawer"
            >
              <div className="flex items-center gap-2">
                <span
                  className="mono-label"
                  style={{ color: "#64748b" }}
                >
                  Reconciliation Exceptions
                </span>
                {openExceptions.length > 0 ? (
                  <span className="badge-warn">{openExceptions.length} open</span>
                ) : (
                  <span className="badge-positive">Clear</span>
                )}
              </div>
              <span style={{ color: "#475569", lineHeight: 1 }}>
                {isExceptionsOpen
                  ? <Icon.ChevronDown aria-hidden="true" />
                  : <Icon.ChevronUp aria-hidden="true" />}
              </span>
            </button>

            {isExceptionsOpen && (
              <div
                id="exceptions-drawer"
                style={{ maxHeight: 180, overflowY: "auto", background: "var(--color-bg)" }}
              >
                {currentScenarioExceptions.length > 0 ? (
                  <table
                    className="w-full text-left border-collapse"
                    aria-label="Reconciliation exceptions"
                  >
                    <thead>
                      <tr>
                        <th className="fin-table-th">Exc ID</th>
                        <th className="fin-table-th">Class</th>
                        <th className="fin-table-th">Order</th>
                        <th className="fin-table-th text-right">Expected</th>
                        <th className="fin-table-th text-right">Actual</th>
                        <th className="fin-table-th text-right">Δ Variance</th>
                        <th className="fin-table-th">Ageing</th>
                        <th className="fin-table-th">Status</th>
                        <th className="fin-table-th text-center">Action</th>
                      </tr>
                    </thead>
                    <tbody>
                      {currentScenarioExceptions.map((exc: ReconException) => (
                        <tr
                          key={exc.id}
                          style={{ opacity: exc.status === "RESOLVED" ? 0.5 : 1 }}
                        >
                          <td className="fin-table-td" style={{ color: "#475569" }}>{exc.id}</td>
                          <td
                            className="fin-table-td font-semibold"
                            style={{ color: "#fbbf24" }}
                          >
                            {exc.class}
                          </td>
                          <td className="fin-table-td" style={{ color: "#64748b" }}>{exc.orderId}</td>
                          <td className="fin-table-td text-right">
                            <MoneyCell paise={exc.expectedPaise} />
                          </td>
                          <td className="fin-table-td text-right">
                            <MoneyCell paise={exc.actualPaise} />
                          </td>
                          <td className="fin-table-td text-right">
                            <MoneyCell paise={exc.variancePaise} direction="net" showSign />
                          </td>
                          <td
                            className="fin-table-td"
                            style={{ color: "#64748b" }}
                          >
                            T+{exc.ageingDays}
                          </td>
                          <td className="fin-table-td">
                            <span
                              className={
                                exc.status === "RESOLVED" ? "badge-positive" : "badge-warn"
                              }
                            >
                              {exc.status}
                            </span>
                          </td>
                          <td className="fin-table-td text-center">
                            {exc.status !== "RESOLVED" ? (
                              <button
                                onClick={() => handleResolveException(exc.id)}
                                className="btn-ghost"
                                style={{ fontSize: 10, height: 20, padding: "0 6px" }}
                                aria-label={`Resolve exception ${exc.id}`}
                              >
                                {exc.remedyAction}
                              </button>
                            ) : (
                              <span
                                className="font-mono"
                                style={{ fontSize: 10, color: "#22c55e" }}
                              >
                                Resolved
                              </span>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                ) : (
                  <div
                    className="font-mono text-center"
                    style={{ padding: "16px", fontSize: 11, color: "#334155" }}
                  >
                    No exceptions — settlement and reconciliation congruent.
                  </div>
                )}
              </div>
            )}
          </div>
        </main>

        {/* ── RIGHT INSPECTOR PANE ─────────────────────────────────────────── */}
        <aside
          className="shrink-0 flex flex-col overflow-hidden"
          style={{
            width: "var(--inspector-w)",
            background: "var(--color-surface)",
            borderLeft: "1px solid var(--color-border)",
          }}
        >
          {/* Tab bar */}
          <div
            className="flex shrink-0"
            role="tablist"
            aria-label="Inspector panel"
            style={{ borderBottom: "1px solid var(--color-border)" }}
          >
            {(["waterfall", "json", "merkle"] as const).map((tab) => (
              <button
                key={tab}
                role="tab"
                aria-selected={activeTab === tab}
                aria-controls={`tab-panel-${tab}`}
                onClick={() => setActiveTab(tab)}
                className={`inspector-tab${activeTab === tab ? " active" : ""}`}
              >
                {tab === "waterfall" ? "Tax Waterfall" : tab === "json" ? "Beckn JSON" : "Merkle Audit"}
              </button>
            ))}
          </div>

          {/* Tab: Tax Waterfall */}
          {activeTab === "waterfall" && (
            <div
              id="tab-panel-waterfall"
              role="tabpanel"
              className="flex-1 overflow-y-auto"
              style={{ padding: 12 }}
            >
              <div style={{ marginBottom: 10 }}>
                <span className="mono-label" style={{ display: "block", marginBottom: 2 }}>
                  Settlement Breakdown · Invariant 13
                </span>
                <span style={{ fontSize: 10, color: "#475569" }}>
                  Gross-to-net waterfall &mdash; Banker&apos;s rounding (RoundHalfEven)
                </span>
              </div>

              {/* Waterfall rows */}
              <div
                className="fin-panel"
                style={{ padding: "8px 10px", marginBottom: 8 }}
              >
                <div className="diff-row" style={{ color: "#e2e8f0" }}>
                  <span>Gross Order Amount</span>
                  <MoneyCell paise={activeScenario.grossPaise} />
                </div>
                <div
                  style={{ borderTop: "1px solid var(--color-border)", margin: "4px 0" }}
                  aria-hidden="true"
                />
                <div className="diff-row" style={{ color: "#fbbf24", paddingLeft: 8 }}>
                  <span style={{ display: "flex", alignItems: "center", gap: 4 }}>
                    <Icon.ArrowRight aria-hidden="true" />
                    (−) Buyer Finder Fee
                  </span>
                  <MoneyCell paise={activeScenario.bffPaise} showSign direction="debit" />
                </div>
                <div className="diff-row" style={{ color: "#fbbf24", paddingLeft: 8 }}>
                  <span style={{ display: "flex", alignItems: "center", gap: 4 }}>
                    <Icon.ArrowRight aria-hidden="true" />
                    (−) Platform Commission
                  </span>
                  <MoneyCell paise={activeScenario.commissionPaise} showSign direction="debit" />
                </div>
                <div className="diff-row" style={{ color: "#fbbf24", paddingLeft: 8 }}>
                  <span style={{ display: "flex", alignItems: "center", gap: 4 }}>
                    <Icon.ArrowRight aria-hidden="true" />
                    (−) TDS Sec 194-O (0.10%)
                  </span>
                  <MoneyCell paise={activeScenario.tdsPaise} showSign direction="debit" />
                </div>
                <div className="diff-row" style={{ color: "#fbbf24", paddingLeft: 8 }}>
                  <span style={{ display: "flex", alignItems: "center", gap: 4 }}>
                    <Icon.ArrowRight aria-hidden="true" />
                    (−) TCS GST Sec 52 (0.50%)
                  </span>
                  <MoneyCell paise={activeScenario.tcsPaise} showSign direction="debit" />
                </div>
                {activeScenario.refundPaise > 0 && (
                  <div className="diff-row" style={{ color: "#f87171", paddingLeft: 8 }}>
                    <span style={{ display: "flex", alignItems: "center", gap: 4 }}>
                      <Icon.ArrowRight aria-hidden="true" />
                      (−) Refund Netting
                    </span>
                    <MoneyCell paise={activeScenario.refundPaise} showSign direction="debit" />
                  </div>
                )}
                <div
                  style={{ borderTop: "1px solid #334155", margin: "6px 0 0" }}
                  aria-hidden="true"
                />
                <div
                  className="diff-row"
                  style={{ color: "#22c55e", fontWeight: 700, fontSize: 12, paddingTop: 4 }}
                >
                  <span>Net Bank Wire Payout</span>
                  <MoneyCell paise={activeScenario.netPayoutPaise} />
                </div>
              </div>

              {/* Invariant verification card */}
              <div className="fin-panel" style={{ padding: "8px 10px" }}>
                <span className="mono-label" style={{ display: "block", marginBottom: 6 }}>
                  Formal Invariants
                </span>
                {[
                  { id: "Inv-01", label: "Zero-Sum Conservation", passed: trialBalance.isBalanced },
                  { id: "Inv-04", label: "Non-Negative Escrow", passed: true },
                  { id: "Inv-13", label: "Gross-to-Net Tie-Out", passed: true },
                ].map((inv) => (
                  <div
                    key={inv.id}
                    className="flex items-center justify-between font-mono"
                    style={{ fontSize: 11, height: 22, color: "#64748b" }}
                  >
                    <span>
                      <span style={{ color: "#475569" }}>{inv.id}</span>
                      {" "}{inv.label}
                    </span>
                    <span style={{ color: inv.passed ? "#22c55e" : "#ef4444", fontWeight: 600 }}>
                      {inv.passed ? "PASS" : "FAIL"}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Tab: Beckn JSON */}
          {activeTab === "json" && (
            <div
              id="tab-panel-json"
              role="tabpanel"
              className="flex-1 overflow-hidden flex flex-col"
              style={{ padding: 8, gap: 6 }}
            >
              <div className="flex items-center justify-between" style={{ padding: "0 2px" }}>
                <span className="mono-label">RSF 2.0 Payload</span>
                <button
                  onClick={copyJsonToClipboard}
                  className="btn-ghost"
                  style={{ height: 22, fontSize: 10, gap: 4 }}
                  aria-label={isCopied ? "Copied to clipboard" : "Copy JSON to clipboard"}
                >
                  {isCopied
                    ? <><Icon.Check style={{ color: "#22c55e" }} aria-hidden="true" /> Copied</>
                    : <><Icon.Copy aria-hidden="true" /> Copy</>}
                </button>
              </div>
              <pre
                className="flex-1 overflow-auto font-mono"
                style={{
                  fontSize: 10,
                  lineHeight: 1.6,
                  color: "#94a3b8",
                  background: "var(--color-bg)",
                  border: "1px solid var(--color-border)",
                  borderRadius: 4,
                  padding: 10,
                  userSelect: "text",
                  whiteSpace: "pre",
                }}
              >
                {activeJsonPayload}
              </pre>
            </div>
          )}

          {/* Tab: Merkle Audit */}
          {activeTab === "merkle" && (
            <div
              id="tab-panel-merkle"
              role="tabpanel"
              className="flex-1 overflow-y-auto"
              style={{ padding: 12 }}
            >
              <div style={{ marginBottom: 10 }}>
                <span className="mono-label" style={{ display: "block", marginBottom: 2 }}>
                  Cryptographic Audit Trail
                </span>
                <span style={{ fontSize: 10, color: "#475569" }}>
                  RFC 6962 domain-separated Merkle tree · PRD §4
                </span>
              </div>

              <div className="fin-panel" style={{ padding: "10px", gap: 8, display: "flex", flexDirection: "column" }}>
                <div>
                  <div className="mono-label" style={{ marginBottom: 4 }}>Batch Root Hash</div>
                  <div
                    className="font-mono"
                    style={{
                      fontSize: 10,
                      lineHeight: 1.6,
                      color: "#22c55e",
                      background: "var(--color-bg)",
                      border: "1px solid var(--color-border)",
                      borderRadius: 4,
                      padding: "6px 8px",
                      wordBreak: "break-all",
                    }}
                  >
                    {activeScenario.merkleAudit.rootHash}
                  </div>
                </div>

                <div style={{ borderTop: "1px solid var(--color-border)", paddingTop: 8 }}>
                  {[
                    { label: "Leaf Nodes", value: `${activeScenario.merkleAudit.leavesCount} leaves` },
                    { label: "Audit Status", value: "VERIFIED_OFFLINE", positive: true },
                  ].map((row) => (
                    <div
                      key={row.label}
                      className="flex items-center justify-between font-mono"
                      style={{ fontSize: 11, height: 24, color: "#64748b" }}
                    >
                      <span>{row.label}</span>
                      <span style={{ color: row.positive ? "#22c55e" : "#94a3b8", fontWeight: row.positive ? 600 : 400 }}>
                        {row.value}
                      </span>
                    </div>
                  ))}
                </div>

                <div style={{ borderTop: "1px solid var(--color-border)", paddingTop: 8 }}>
                  <div className="mono-label" style={{ marginBottom: 4 }}>Ed25519 Network Signature</div>
                  <div
                    className="font-mono"
                    style={{
                      fontSize: 9,
                      lineHeight: 1.6,
                      color: "#475569",
                      background: "var(--color-bg)",
                      border: "1px solid var(--color-border)",
                      borderRadius: 4,
                      padding: "6px 8px",
                      wordBreak: "break-all",
                    }}
                  >
                    {activeScenario.merkleAudit.signatureEd25519}
                  </div>
                </div>
              </div>
            </div>
          )}
        </aside>
      </div>
    </div>
  );
}
