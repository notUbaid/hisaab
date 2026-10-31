"use client";

import React, { useState } from "react";
import {
  FileText,
  Bookmark,
  ShieldCheck,
  CheckCircle2,
  AlertTriangle,
  GitBranch,
  Layers,
  Search,
  ExternalLink,
  Sliders,
} from "lucide-react";

interface AmbiguityItem {
  id: string;
  edgeCaseId: string;
  title: string;
  brdRef: string;
  description: string;
  policyFlag: string;
  defaultPolicy: string;
  options: { name: string; behavior: string }[];
}

const AMBIGUITIES: AmbiguityItem[] = [
  {
    id: "AMB-001",
    edgeCaseId: "EC-08",
    title: "Match Semantics: Order ID only vs Order ID + Component Amounts",
    brdRef: "BRD §4.2 & Annexure Balance Check Example",
    description:
      "Whether the SA matches strictly on Network Order ID alone and routes component discrepancies to recon, or requires an exact 0-paise match across all fee components.",
    policyFlag: "match_amount_tolerance_paise",
    defaultPolicy: "strict_all_components (0 paise tolerance)",
    options: [
      {
        name: "strict_all_components",
        behavior: "Order marked amount_mismatch unless order amount, commission, and BFF match exactly (0 paise).",
      },
      {
        name: "order_id_only",
        behavior: "Order matched on ID alone; component discrepancies routed to recon exception queue.",
      },
    ],
  },
  {
    id: "AMB-002",
    edgeCaseId: "EC-12",
    title: "Insufficient Balance: All-or-Nothing vs Partial Fill",
    brdRef: "BRD §4.2 Case 12",
    description:
      "When Collector's settlement pool lacks sufficient funds, whether to abort the entire batch or settle orders via FIFO priority.",
    policyFlag: "partial_fill_allowed",
    defaultPolicy: "all_or_nothing (Zero risk of preferential creditor preference)",
    options: [
      {
        name: "all_or_nothing",
        behavior: "Reject all matched orders in batch; no funds move; Collector notified to top up.",
      },
      {
        name: "fifo_partial_fill",
        behavior: "Settle orders in ascending order until balance exhausted; reject remainder.",
      },
    ],
  },
  {
    id: "AMB-003",
    edgeCaseId: "EC-10",
    title: "Carry-Forward Expiration and Retention Horizon",
    brdRef: "BRD §4.2 Cases 9, 10, 11",
    description:
      "How long unmatched counterparty claims remain active before automated expiry and unfreezing of collateral.",
    policyFlag: "carry_forward_days",
    defaultPolicy: "2_business_days (T+2 Escalation Threshold)",
    options: [
      {
        name: "1_business_day",
        behavior: "Hold for T+1 only; expire at T+1 cut-off.",
      },
      {
        name: "2_business_days",
        behavior: "Hold across T+1 and T+2 cycles; escalate to operational exception triage if unresolved.",
      },
    ],
  },
  {
    id: "AMB-004",
    edgeCaseId: "EC-14",
    title: "Tax Jurisdiction Determination: Place of Supply vs Billing Address",
    brdRef: "BRD §4.2 Tax Withholding & Section 52 CGST Act",
    description:
      "Whether GST TCS applies as Intra-State (CGST 0.25% + SGST 0.25%) or Inter-State (IGST 0.50%) based on Buyer shipping address or Seller principal business address.",
    policyFlag: "tax_pos_source",
    defaultPolicy: "shipping_address (Section 10 IGST Act 2017)",
    options: [
      {
        name: "shipping_address",
        behavior: "Place of Supply is destination address where goods delivered.",
      },
      {
        name: "bpp_registered_state",
        behavior: "Place of Supply defaults to seller state.",
      },
    ],
  },
];

const SPEC_PINS = [
  {
    name: "Automation Specifications",
    repo: "ONDC-Official/automation-specifications",
    branch: "master",
    sha: "c6a7e8b912345678",
    specVersion: "NTS10-2.0.0",
    pinnedAt: "2026-10-30T00:00:00Z",
  },
  {
    name: "Log Validation Utility",
    repo: "ONDC-Official/log-validation-utility",
    branch: "main",
    sha: "a1b2c3d4e5f67890",
    specVersion: "RSF-2.0",
    pinnedAt: "2026-10-30T00:00:00Z",
  },
  {
    name: "Mock Server Utility",
    repo: "ONDC-Official/mock-server-utility",
    branch: "main",
    sha: "e9f8d7c6b5a43210",
    specVersion: "RSF-2.0",
    pinnedAt: "2026-10-30T00:00:00Z",
  },
  {
    name: "RSF Bulk Settlement Utility",
    repo: "ONDC-Official/rsf-bulk-utility",
    branch: "main",
    sha: "1234567890abcdef",
    specVersion: "RSF-2.0",
    pinnedAt: "2026-10-30T00:00:00Z",
  },
];

export function SpecInspector({ className = "" }: { className?: string }) {
  const [activeTab, setActiveTab] = useState<"ambiguities" | "pins">("ambiguities");
  const [searchQuery, setSearchQuery] = useState("");

  const filteredAmbiguities = AMBIGUITIES.filter(
    (a) =>
      !searchQuery ||
      a.id.toLowerCase().includes(searchQuery.toLowerCase()) ||
      a.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
      a.policyFlag.toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div className={`fin-card overflow-hidden ${className}`}>
      <div className="px-4 py-3 bg-obsidian-900/90 border-b border-white/5 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Sliders className="w-4 h-4 text-cyan-400" />
          <span className="font-semibold text-xs text-white uppercase tracking-wider">
            ONDC RSF 2.0 Spec & Policy Matrix (spec/ambiguities.yaml & spec/ondc-pins.yaml)
          </span>
        </div>

        <div className="flex items-center gap-2 text-xs font-mono">
          <button
            onClick={() => setActiveTab("ambiguities")}
            className={`px-3 py-1 rounded-md transition ${
              activeTab === "ambiguities"
                ? "bg-cyan-500/20 text-cyan-300 border border-cyan-500/40"
                : "text-slate-400 hover:text-white"
            }`}
          >
            Ambiguity Register ({AMBIGUITIES.length})
          </button>
          <button
            onClick={() => setActiveTab("pins")}
            className={`px-3 py-1 rounded-md transition ${
              activeTab === "pins"
                ? "bg-cyan-500/20 text-cyan-300 border border-cyan-500/40"
                : "text-slate-400 hover:text-white"
            }`}
          >
            Upstream Git Pins ({SPEC_PINS.length})
          </button>
        </div>
      </div>

      <div className="p-4 space-y-4">
        {activeTab === "ambiguities" && (
          <div className="space-y-3 font-mono">
            <div className="flex items-center gap-2 max-w-sm">
              <Search className="w-3.5 h-3.5 text-slate-500" />
              <input
                type="text"
                placeholder="Search ambiguities or policy flags..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full bg-obsidian-900 border border-white/10 rounded px-2.5 py-1 text-xs text-white placeholder:text-slate-500 focus:outline-none focus:border-cyan-500/50"
              />
            </div>

            <div className="space-y-3">
              {filteredAmbiguities.map((item) => (
                <div
                  key={item.id}
                  className="rounded-lg bg-obsidian-900 border border-white/5 p-3.5 space-y-2 hover:border-white/10 transition"
                >
                  <div className="flex items-start justify-between gap-2">
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="pill-cyan text-[10px] font-bold">{item.id}</span>
                        <span className="text-[10px] text-slate-400">{item.edgeCaseId}</span>
                        <h3 className="font-sans text-xs font-semibold text-white">
                          {item.title}
                        </h3>
                      </div>
                      <div className="text-[10px] text-slate-500 mt-0.5">
                        Citation: {item.brdRef}
                      </div>
                    </div>

                    <div className="text-right shrink-0">
                      <span className="text-[9px] uppercase text-slate-500 block">Flag</span>
                      <span className="px-2 py-0.5 rounded bg-white/5 border border-white/5 text-[10px] text-amber-300">
                        {item.policyFlag}
                      </span>
                    </div>
                  </div>

                  <p className="font-sans text-xs text-slate-300 leading-relaxed">
                    {item.description}
                  </p>

                  <div className="pt-2 border-t border-white/5 grid grid-cols-1 md:grid-cols-2 gap-2 text-[11px]">
                    {item.options.map((opt, i) => {
                      const isDefault = item.defaultPolicy.includes(opt.name);
                      return (
                        <div
                          key={i}
                          className={`p-2 rounded border ${
                            isDefault
                              ? "bg-cyan-500/10 border-cyan-500/30 text-slate-200"
                              : "bg-obsidian-950/60 border-white/5 text-slate-400"
                          }`}
                        >
                          <div className="flex items-center justify-between font-bold mb-0.5">
                            <span className={isDefault ? "text-cyan-300" : "text-slate-300"}>
                              {opt.name}
                            </span>
                            {isDefault && (
                              <span className="text-[9px] uppercase px-1 rounded bg-cyan-500/20 text-cyan-300">
                                Active Default
                              </span>
                            )}
                          </div>
                          <div className="font-sans text-[11px] leading-normal">{opt.behavior}</div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {activeTab === "pins" && (
          <div className="space-y-3 font-mono">
            <p className="font-sans text-xs text-slate-400">
              Hisaab pins upstream ONDC repositories with exact Git commit SHAs to detect breaking schema changes and tax formula updates automatically.
            </p>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              {SPEC_PINS.map((pin, idx) => (
                <div
                  key={idx}
                  className="rounded-lg bg-obsidian-900 border border-white/5 p-3 space-y-2 hover:border-white/10 transition"
                >
                  <div className="flex items-center justify-between">
                    <span className="font-sans text-xs font-semibold text-white">
                      {pin.name}
                    </span>
                    <span className="pill-emerald text-[9px]">PINNED</span>
                  </div>

                  <div className="space-y-1 text-xs">
                    <div className="flex items-center justify-between text-slate-400">
                      <span className="text-slate-500">Repository:</span>
                      <span className="text-slate-300">{pin.repo}</span>
                    </div>
                    <div className="flex items-center justify-between text-slate-400">
                      <span className="text-slate-500">Branch:</span>
                      <span className="text-cyan-400 flex items-center gap-1">
                        <GitBranch className="w-3 h-3" /> {pin.branch}
                      </span>
                    </div>
                    <div className="flex items-center justify-between text-slate-400">
                      <span className="text-slate-500">Commit SHA:</span>
                      <span className="text-purple-300 font-bold">{pin.sha}</span>
                    </div>
                    <div className="flex items-center justify-between text-slate-400">
                      <span className="text-slate-500">Spec Version:</span>
                      <span className="text-emerald-400">{pin.specVersion}</span>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
