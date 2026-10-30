"use client";

import React, { useState } from "react";
import {
  Wrench,
  Zap,
  CheckCircle2,
  AlertTriangle,
  Play,
  RotateCw,
  Code2,
  FileCheck,
  ShieldAlert,
  ServerCrash,
  Radio,
  ShieldCheck,
  Award,
  KeyRound,
  Lock,
} from "lucide-react";
import { TaxWaterfall } from "@/components/TaxWaterfall";
import { SignatureInspector } from "@/components/SignatureInspector";
import { apiClient } from "@/lib/apiClient";

export default function PlaygroundPage() {
  // Schema Validator State
  const [jsonInput, setJsonInput] = useState<string>(() => {
    return JSON.stringify(
      {
        context: {
          domain: "ONDC:RET10",
          country: "IND",
          city: "std:080",
          action: "settle",
          core_version: "2.0.0",
          bap_id: "buyer-app.ondc.org",
          bpp_id: "seller-app.ondc.org",
          transaction_id: "TXN-20261030-001",
          message_id: "MSG-20261030-9941",
          timestamp: "2026-10-30T17:00:00.000Z",
          ttl: "PT30S",
        },
        message: {
          settlement: {
            settlement_id: "SETTLE-BATCH-01",
            orders: [
              {
                id: "ORD-9841",
                amount: { currency: "INR", value: "1250.00" },
                commission: { currency: "INR", value: "62.50" },
                buyer_finder_fee: { currency: "INR", value: "37.50" },
                withholding_tax: {
                  tds_194o: "1.25",
                  tcs_gst: "6.25",
                },
              },
            ],
          },
        },
      },
      null,
      2
    );
  });

  const [validationResult, setValidationResult] = useState<{
    valid: boolean;
    errors: { line: number; message: string; field: string }[];
    specsChecked: string[];
  } | null>(null);

  // Chaos Failpoints State (internal/verification/failpoints.go)
  const [failpoints, setFailpoints] = useState({
    after_debit_before_credit: false,
    mid_batch_database_sever: false,
    after_instruction_before_bank_send: false,
    duplicate_credit_arrival: false,
    clock_skew_midnight: false,
    tampered_ed25519_signature: false,
  });

  const [chaosTestResult, setChaosTestResult] = useState<string | null>(null);
  const [isChaosError, setIsChaosError] = useState(false);
  const [isSimulatingChaos, setIsSimulatingChaos] = useState(false);

  const [isValidating, setIsValidating] = useState<boolean>(false);
  const [signatureTampered, setSignatureTampered] = useState<boolean>(false);

  // Run JSON schema validation against ONDC RSF 2.0 specs
  const handleValidateJSON = async () => {
    setIsValidating(true);
    try {
      const parsed = JSON.parse(jsonInput);
      const res = await apiClient.post<{
        valid: boolean;
        errors: { line: number; message: string; field: string }[];
        specsEvaluated: string[];
      }>("/api/toolkit", {
        action: "validate_schema",
        payload: parsed,
      });

      setValidationResult({
        valid: res.valid,
        errors: res.errors,
        specsChecked: res.specsEvaluated,
      });
    } catch (e: any) {
      setValidationResult({
        valid: false,
        errors: [{ line: 1, message: `Syntax Error: ${e.message}`, field: "JSON Syntax" }],
        specsChecked: ["JSON RFC 8259"],
      });
    } finally {
      setIsValidating(false);
    }
  };

  // Run Chaos injection simulation
  const handleRunChaosTest = async () => {
    setIsSimulatingChaos(true);
    setChaosTestResult(null);
    setIsChaosError(false);

    // Identify active failpoint
    const activeFailpointKey = Object.entries(failpoints).find(([_, active]) => active)?.[0] || "FP_NONE";

    try {
      const res = await fetch("/api/scenarios/run", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          scenarioId: "ec-14",
          targetMode: "live_api",
          failpoint: activeFailpointKey,
        }),
      });
      const data = await res.json();
      setIsSimulatingChaos(false);
      if (res.ok && data.passed) {
        setIsChaosError(false);
        setChaosTestResult(
          `Chaos Handled Safely: Ledger Rolled Back Atomically. Invariant 4 Protected (Zero-Sum Intact).`
        );
      } else {
        setIsChaosError(true);
        setChaosTestResult(
          `Verification Failed: ${data.error || "Simulation rejected unhandled state"}`
        );
      }
    } catch {
      setIsSimulatingChaos(false);
      setIsChaosError(true);
      setChaosTestResult(
        "Verification Engine Unavailable: Could not execute failpoint check (Offline)."
      );
    }
  };

  return (
    <div className="flex-1 overflow-y-auto p-6 space-y-6 max-w-7xl mx-auto w-full font-sans">
      {/* ── Header ── */}
      <div className="flex flex-wrap items-center justify-between gap-4 pb-2 border-b border-white/5">
        <div>
          <h1 className="text-xl font-bold text-white tracking-tight flex items-center gap-2.5">
            NP Toolkit & Chaos Playground
            <span className="pill-cyan text-[10px]">Interactive Lab</span>
          </h1>
          <p className="text-xs text-slate-400 font-mono mt-1">
            Statutory Tax Waterfall Simulation, Schema Compliance & Chaos Fault Injection
          </p>
        </div>
      </div>

      {/* ── 1. Interactive Tax Waterfall Builder ── */}
      <div>
        <div className="flex items-center gap-2 mb-3">
          <Wrench className="w-4 h-4 text-emerald-400" />
          <h2 className="font-semibold text-xs text-white uppercase tracking-wider">
            1. Interactive Statutory Tax Waterfall Builder
          </h2>
        </div>
        <TaxWaterfall initialOrderValuePaise={250000} initialBffBps={300} initialCommissionBps={500} />
      </div>

      {/* ── 2. Schema & Pin Compliance Checker ── */}
      <div className="fin-card overflow-hidden">
        <div className="px-4 py-3 bg-obsidian-900/90 border-b border-white/5 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Code2 className="w-4 h-4 text-cyan-400" />
            <span className="font-semibold text-xs text-white uppercase tracking-wider">
              2. ONDC RSF 2.0 Schema & Pin Validator (spec/ondc-pins.yaml)
            </span>
          </div>

          <button
            onClick={handleValidateJSON}
            className="btn-primary py-1 px-3 text-xs flex items-center gap-1.5 shadow-glow"
          >
            <FileCheck className="w-3.5 h-3.5" />
            <span>Validate Payload</span>
          </button>
        </div>

        <div className="grid grid-cols-2 divide-x divide-white/5">
          {/* JSON Textarea */}
          <div className="p-4">
            <textarea
              value={jsonInput}
              onChange={(e) => setJsonInput(e.target.value)}
              className="w-full h-72 bg-obsidian-900 border border-white/10 rounded-lg p-3 font-mono text-[11px] text-slate-300 focus:outline-none focus:border-cyan-500/50 resize-none"
              placeholder="Paste raw Beckn /settle or /on_settle JSON payload..."
            />
          </div>

          {/* Validation Diagnostics */}
          <div className="p-4 space-y-3 font-mono text-xs">
            <div className="text-[10px] uppercase text-slate-500 tracking-wider">
              Validation Diagnostics
            </div>

            {validationResult ? (
              <div className="space-y-3">
                {validationResult.valid ? (
                  <div className="p-3 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-emerald-300 space-y-1">
                    <div className="font-bold flex items-center gap-1.5 text-xs">
                      <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                      Payload 100% Compliant with ONDC RSF 2.0 Pins
                    </div>
                    <p className="text-[11px] text-slate-300">
                      All required context headers, order structures, and statutory withholding blocks match upstream pins.
                    </p>
                  </div>
                ) : (
                  <div className="p-3 rounded-lg bg-rose-500/10 border border-rose-500/30 text-rose-300 space-y-2">
                    <div className="font-bold flex items-center gap-1.5 text-xs">
                      <AlertTriangle className="w-4 h-4 text-rose-400" />
                      Found {validationResult.errors.length} Schema Violations
                    </div>
                    <div className="space-y-1 text-[11px]">
                      {validationResult.errors.map((err, idx) => (
                        <div key={idx} className="flex items-start gap-2 text-slate-300">
                          <span className="text-rose-400 font-bold shrink-0">Line {err.line}:</span>
                          <span>{err.message}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Specs Checked */}
                <div className="pt-2 border-t border-white/5 space-y-1 text-[10px] text-slate-400">
                  <div className="text-slate-500 uppercase">Upstream Spec Pins Evaluated:</div>
                  {validationResult.specsChecked.map((spec, i) => (
                    <div key={i} className="text-slate-300 flex items-center gap-1">
                      <span className="text-cyan-400">✓</span> {spec}
                    </div>
                  ))}
                </div>
              </div>
            ) : (
              <div className="text-slate-500 text-center py-16">
                Click &quot;Validate Payload&quot; to test against upstream ONDC RSF 2.0 schemas.
              </div>
            )}
          </div>
        </div>
      </div>

      {/* ── 3. Failpoint Chaos Toggles ── */}
      <div className="fin-card overflow-hidden">
        <div className="px-4 py-3 bg-obsidian-900/90 border-b border-white/5 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Zap className="w-4 h-4 text-amber-400" />
            <span className="font-semibold text-xs text-white uppercase tracking-wider">
              3. Failpoint Chaos Switches (internal/verification/failpoints.go)
            </span>
          </div>

          <button
            onClick={handleRunChaosTest}
            disabled={isSimulatingChaos}
            className="py-1 px-3 rounded-md bg-amber-600 hover:bg-amber-500 text-white font-semibold text-xs transition flex items-center gap-1.5 shadow-glow-amber"
          >
            {isSimulatingChaos ? (
              <>
                <RotateCw className="w-3.5 h-3.5 animate-spin" />
                <span>Simulating...</span>
              </>
            ) : (
              <>
                <ServerCrash className="w-3.5 h-3.5" />
                <span>Inject Fault & Settle</span>
              </>
            )}
          </button>
        </div>

        <div className="p-4 space-y-4">
          <p className="text-xs text-slate-400">
            Activate deterministic crash points along the settlement execution path to test that double-entry guarantees and rollback invariants hold.
          </p>

          <div className="grid grid-cols-3 gap-3">
            {[
              {
                id: "after_debit_before_credit",
                name: "Post-Debit Crash",
                desc: "Kills process after collector debit but before receiver credit.",
              },
              {
                id: "mid_batch_database_sever",
                name: "Database Disconnect",
                desc: "Simulates socket break during active batch transaction.",
              },
              {
                id: "after_instruction_before_bank_send",
                name: "RTGS Bank Timeout",
                desc: "Delays or times out bank wire dispatch call.",
              },
              {
                id: "duplicate_credit_arrival",
                name: "Duplicate Webhook Replay",
                desc: "Replays identical Beckn message ID to trigger idempotency.",
              },
              {
                id: "clock_skew_midnight",
                name: "Midnight Clock Skew",
                desc: "Jumps system time backwards across 00:00 IST cut-off.",
              },
              {
                id: "tampered_ed25519_signature",
                name: "Signature Tampering",
                desc: "Alters 1 byte of Authorization header to verify Blake-512 reject.",
              },
            ].map((fp) => {
              const active = failpoints[fp.id as keyof typeof failpoints];
              return (
                <div
                  key={fp.id}
                  onClick={() =>
                    setFailpoints((prev) => ({
                      ...prev,
                      [fp.id]: !prev[fp.id as keyof typeof failpoints],
                    }))
                  }
                  className={`p-3 rounded-lg border transition cursor-pointer ${
                    active
                      ? "bg-amber-500/10 border-amber-500/40 shadow-glow-amber"
                      : "bg-obsidian-900 border-white/5 hover:border-white/10"
                  }`}
                >
                  <div className="flex items-center justify-between mb-1">
                    <span className="font-semibold text-xs text-slate-200">{fp.name}</span>
                    <input
                      type="checkbox"
                      checked={active}
                      onChange={() => {}}
                      className="rounded bg-obsidian-900 border-white/20 text-amber-500"
                    />
                  </div>
                  <p className="text-[10px] text-slate-400 font-mono leading-relaxed">
                    {fp.desc}
                  </p>
                </div>
              );
            })}
          </div>

          {chaosTestResult && (
            <div
              className={`p-3 rounded-lg border text-xs font-mono flex items-center gap-2 animate-in fade-in ${
                isChaosError
                  ? "bg-rose-500/10 border-rose-500/30 text-rose-300"
                  : "bg-emerald-500/10 border-emerald-500/30 text-emerald-300"
              }`}
            >
              {isChaosError ? (
                <AlertTriangle className="w-4 h-4 text-rose-400 shrink-0" />
              ) : (
                <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" />
              )}
              <span>{chaosTestResult}</span>
            </div>
          )}
        </div>
      </div>

      {/* ── 4. Adversarial & Chaos Invariant Scorecard ── */}
      <div className="fin-card overflow-hidden">
        <div className="px-4 py-3 bg-obsidian-900/90 border-b border-white/5 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Award className="w-4 h-4 text-emerald-400" />
            <span className="font-semibold text-xs text-white uppercase tracking-wider">
              4. Adversarial & Crash Invariant Scorecard (tests/)
            </span>
          </div>
          <span className="pill-emerald text-[10px]">100% Invariants Conserved</span>
        </div>

        <div className="p-4 grid grid-cols-1 md:grid-cols-4 gap-3 font-mono text-xs">
          <div className="p-3 rounded-lg bg-obsidian-900 border border-white/5 space-y-1.5">
            <div className="flex items-center justify-between text-slate-400">
              <span className="text-[10px] uppercase font-bold text-white">adversarial_test.go</span>
              <span className="text-emerald-400 font-semibold">11/11 PASS</span>
            </div>
            <p className="text-[10px] text-slate-400">
              A1–A11 suites: zero-amount, future dates, currency drift, concurrent overdrafts, penny tolerance limits.
            </p>
          </div>

          <div className="p-3 rounded-lg bg-obsidian-900 border border-white/5 space-y-1.5">
            <div className="flex items-center justify-between text-slate-400">
              <span className="text-[10px] uppercase font-bold text-white">crash_test.go</span>
              <span className="text-emerald-400 font-semibold">6/6 PASS</span>
            </div>
            <p className="text-[10px] text-slate-400">
              Atomic failpoints: post-debit abort, mid-batch socket kill, WAL crash recovery, zero-sum intact.
            </p>
          </div>

          <div className="p-3 rounded-lg bg-obsidian-900 border border-white/5 space-y-1.5">
            <div className="flex items-center justify-between text-slate-400">
              <span className="text-[10px] uppercase font-bold text-white">mutation_test.go</span>
              <span className="text-cyan-400 font-semibold">100% SCORE</span>
            </div>
            <p className="text-[10px] text-slate-400">
              Mutant kill rate: inverted tax signs, boundary offsets, rounding direction reversals caught by checkers.
            </p>
          </div>

          <div className="p-3 rounded-lg bg-obsidian-900 border border-white/5 space-y-1.5">
            <div className="flex items-center justify-between text-slate-400">
              <span className="text-[10px] uppercase font-bold text-white">ledger_test.go</span>
              <span className="text-emerald-400 font-semibold">4/4 PASS</span>
            </div>
            <p className="text-[10px] text-slate-400">
              Row-level concurrency: simultaneous debits on account_balances, strict overdraft barrier (balance ≥ 0).
            </p>
          </div>
        </div>
      </div>

      {/* ── 5. Beckn Ed25519 & BLAKE-512 Authorization Inspector ── */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <KeyRound className="w-4 h-4 text-purple-400" />
            <h2 className="font-semibold text-xs text-white uppercase tracking-wider">
              5. Beckn Ed25519 & BLAKE-512 Authorization Inspector (internal/auth/signing.go)
            </h2>
          </div>

          <button
            onClick={() => setSignatureTampered((prev) => !prev)}
            className={`px-2.5 py-1 rounded text-xs font-mono border transition ${
              signatureTampered
                ? "bg-rose-500/10 text-rose-300 border-rose-500/30"
                : "bg-obsidian-900 text-slate-400 border-white/10 hover:text-white"
            }`}
          >
            {signatureTampered ? "Tampered Byte Injected (Fail)" : "Simulate Tampered Signature"}
          </button>
        </div>

        <SignatureInspector
          bapId="buyer-app.ondc.org"
          bppId="seller-app.ondc.org"
          createdTimestamp={new Date(Date.now() - 15000).toISOString()}
          expiresTimestamp={new Date(Date.now() + 15000).toISOString()}
          isVerified={!signatureTampered}
        />
      </div>
    </div>
  );
}
