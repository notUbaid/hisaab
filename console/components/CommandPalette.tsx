"use client";

import React, { useState, useEffect, useMemo, useRef } from "react";
import { useRouter } from "next/navigation";
import {
  Search,
  Command,
  ArrowRight,
  FlaskConical,
  AlertTriangle,
  LayoutDashboard,
  Wrench,
  GitCommit,
  ShieldCheck,
  Download,
  Users,
  FileCode,
  Hash,
  ExternalLink,
  Layers,
  Activity,
  Sliders,
} from "lucide-react";
import { SCENARIOS_DATA } from "@/lib/scenarioData";
import { ParticipantPerspective } from "@/lib/rbac";

export interface CommandPaletteProps {
  isOpen: boolean;
  onClose: () => void;
  onSelectPerspective?: (perspective: ParticipantPerspective) => void;
  onOpenWireModal?: () => void;
  onRunConformanceSuite?: () => void;
  onOpenWorkerMonitor?: () => void;
  onOpenSpecInspector?: () => void;
}

interface PaletteItem {
  id: string;
  category: "Navigation" | "Scenarios" | "Entities" | "Actions";
  title: string;
  subtitle?: string;
  icon: React.ComponentType<{ className?: string }>;
  badge?: string;
  action: () => void;
}

export function CommandPalette({
  isOpen,
  onClose,
  onSelectPerspective,
  onOpenWireModal,
  onRunConformanceSuite,
  onOpenWorkerMonitor,
  onOpenSpecInspector,
}: CommandPaletteProps) {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (isOpen) {
      setQuery("");
      setSelectedIndex(0);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [isOpen]);

  const items = useMemo<PaletteItem[]>(() => {
    const list: PaletteItem[] = [
      // Navigation
      {
        id: "nav-overview",
        category: "Navigation",
        title: "Executive Overview",
        subtitle: "Gross volumes, live T+1 cutoff, multi-party pipeline",
        icon: LayoutDashboard,
        badge: "Route 1",
        action: () => {
          router.push("/");
          onClose();
        },
      },
      {
        id: "nav-exceptions",
        category: "Navigation",
        title: "Exceptions Triage",
        subtitle: "3-way recon diff, variance diagnosis, tolerance approvals",
        icon: AlertTriangle,
        badge: "Route 2",
        action: () => {
          router.push("/exceptions");
          onClose();
        },
      },
      {
        id: "nav-scenarios",
        category: "Navigation",
        title: "Scenarios Studio",
        subtitle: "22 ONDC edge cases, double-entry T-accounts, DSL stepper",
        icon: FlaskConical,
        badge: "Route 3",
        action: () => {
          router.push("/scenarios");
          onClose();
        },
      },
      {
        id: "nav-playground",
        category: "Navigation",
        title: "NP Toolkit & Chaos Playground",
        subtitle: "Tax waterfall, schema validator, fault injection switches",
        icon: Wrench,
        badge: "Route 4",
        action: () => {
          router.push("/playground");
          onClose();
        },
      },
      {
        id: "nav-timeline",
        category: "Navigation",
        title: "Cryptographic Audit Timeline",
        subtitle: "RFC 6962 Merkle tree, Ed25519 signatures, vertical subway",
        icon: GitCommit,
        badge: "Route 5",
        action: () => {
          router.push("/timeline");
          onClose();
        },
      },

      // Quick Actions
      {
        id: "action-run-suite",
        category: "Actions",
        title: "Run Full Conformance Suite",
        subtitle: "Execute all 22 ONDC edge-case scenarios in parallel",
        icon: ShieldCheck,
        badge: "Verification",
        action: () => {
          if (onRunConformanceSuite) {
            onRunConformanceSuite();
          } else {
            router.push("/scenarios?runSuite=true");
          }
          onClose();
        },
      },
      {
        id: "action-worker-monitor",
        category: "Actions",
        title: "Launch Worker Queue & Outbox Monitor",
        subtitle: "Real-time Go background settlement outbox telemetry",
        icon: Activity,
        badge: "Telemetry",
        action: () => {
          if (onOpenWorkerMonitor) onOpenWorkerMonitor();
          onClose();
        },
      },
      {
        id: "action-spec-inspector",
        category: "Actions",
        title: "Launch Spec & Policy Ambiguity Matrix",
        subtitle: "Review ONDC RSF 2.0 commit SHA pins and policy toggles",
        icon: Sliders,
        badge: "Spec",
        action: () => {
          if (onOpenSpecInspector) onOpenSpecInspector();
          onClose();
        },
      },
      {
        id: "action-wire-modal",
        category: "Actions",
        title: "Inspect ISO 20022 Wire Batch",
        subtitle: "Review pacs.008 XML/JSON & statutory RBI calendar roll",
        icon: Layers,
        badge: "Settlement",
        action: () => {
          if (onOpenWireModal) {
            onOpenWireModal();
          }
          onClose();
        },
      },
      {
        id: "action-export-exceptions",
        category: "Actions",
        title: "Export Reconciliation CSV",
        subtitle: "Download all active variance items with rule citations",
        icon: Download,
        badge: "Data",
        action: () => {
          router.push("/exceptions?export=csv");
          onClose();
        },
      },
      {
        id: "action-role-collector",
        category: "Actions",
        title: "Switch RBAC Role to Collector RSP",
        subtitle: "Manage nodal account and execute atomic net settlement",
        icon: Users,
        badge: "Role",
        action: () => {
          onSelectPerspective?.("COLLECTOR_RSP");
          onClose();
        },
      },
      {
        id: "action-role-buyer",
        category: "Actions",
        title: "Switch RBAC Role to Buyer App (BAP)",
        subtitle: "Verify BFF collection, buyer refund credits, item invoices",
        icon: Users,
        badge: "Role",
        action: () => {
          onSelectPerspective?.("BUYER_APP_BAP");
          onClose();
        },
      },
      {
        id: "action-role-seller",
        category: "Actions",
        title: "Switch RBAC Role to Seller App (BPP)",
        subtitle: "Inspect net merchant payouts, GST TCS, and TDS 194-O",
        icon: Users,
        badge: "Role",
        action: () => {
          onSelectPerspective?.("SELLER_APP_BPP");
          onClose();
        },
      },

      // Pinned Entities
      {
        id: "entity-ord-9841",
        category: "Entities",
        title: "ORD-9841 (Retail Settlement Batch)",
        subtitle: "Gross ₹1,250.00 · Buyer App ↔ Seller App · RTGS Dispatched",
        icon: Hash,
        badge: "Order",
        action: () => {
          router.push("/exceptions?search=ORD-9841");
          onClose();
        },
      },
      {
        id: "entity-utr-984712",
        category: "Entities",
        title: "UTR9847120391 (HDFC Nodal Escrow Wire)",
        subtitle: "State Bank of India clearing · pacs.008 confirmed",
        icon: FileCode,
        badge: "UTR",
        action: () => {
          router.push("/timeline");
          onClose();
        },
      },
      {
        id: "entity-np-bap",
        category: "Entities",
        title: "buyer-app.ondc.org (Subscriber NP)",
        subtitle: "Network Participant ID: BAP-IND-001 · Ed25519 Registered",
        icon: ExternalLink,
        badge: "Network Participant",
        action: () => {
          router.push("/playground");
          onClose();
        },
      },
    ];

    // Append all 22 scenarios
    SCENARIOS_DATA.forEach((s) => {
      list.push({
        id: `scenario-${s.id.toLowerCase()}`,
        category: "Scenarios",
        title: `${s.id}: ${s.title}`,
        subtitle: `${s.npPair} · ₹${(s.grossPaise / 100).toLocaleString("en-IN")} · ${s.brdSection}`,
        icon: FlaskConical,
        badge: s.status,
        action: () => {
          router.push(`/scenarios?id=${s.id}`);
          onClose();
        },
      });
    });

    return list;
  }, [
    router,
    onClose,
    onSelectPerspective,
    onOpenWireModal,
    onRunConformanceSuite,
    onOpenWorkerMonitor,
    onOpenSpecInspector,
  ]);

  // Filter items by query
  const filteredItems = useMemo(() => {
    if (!query.trim()) return items;
    const q = query.toLowerCase();
    return items.filter(
      (item) =>
        item.title.toLowerCase().includes(q) ||
        item.category.toLowerCase().includes(q) ||
        (item.subtitle && item.subtitle.toLowerCase().includes(q)) ||
        (item.badge && item.badge.toLowerCase().includes(q))
    );
  }, [items, query]);

  // Handle keyboard navigation
  useEffect(() => {
    if (!isOpen) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setSelectedIndex((prev) => (prev + 1 < filteredItems.length ? prev + 1 : 0));
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        setSelectedIndex((prev) => (prev - 1 >= 0 ? prev - 1 : filteredItems.length - 1));
      } else if (e.key === "Enter") {
        e.preventDefault();
        if (filteredItems[selectedIndex]) {
          filteredItems[selectedIndex].action();
        }
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, filteredItems, selectedIndex]);

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-start justify-center pt-20 p-4 animate-in fade-in duration-150"
      onClick={onClose}
    >
      <div
        className="w-full max-w-2xl bg-obsidian-850 border border-white/10 rounded-xl shadow-2xl overflow-hidden font-sans animate-in zoom-in-95 duration-150 flex flex-col max-h-[75vh]"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Search Input Bar */}
        <div className="flex items-center px-4 py-3.5 border-b border-white/10 gap-3 bg-obsidian-900 shrink-0">
          <Search className="w-4 h-4 text-emerald-400 shrink-0" />
          <input
            ref={inputRef}
            type="text"
            placeholder="Type a command, scenario (EC-01..22), entity (ORD, UTR, NP) or action..."
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setSelectedIndex(0);
            }}
            className="w-full bg-transparent text-sm text-white placeholder:text-slate-500 focus:outline-none font-mono"
          />
          <kbd className="px-1.5 py-0.5 rounded bg-white/5 border border-white/10 text-[10px] text-slate-400 font-mono shrink-0">
            ESC
          </kbd>
        </div>

        {/* Results List */}
        <div className="overflow-y-auto p-2 space-y-1">
          {filteredItems.length === 0 ? (
            <div className="py-12 text-center text-slate-500 font-mono text-xs">
              No matching commands or entities found for &quot;{query}&quot;.
            </div>
          ) : (
            filteredItems.map((item, idx) => {
              const isSelected = idx === selectedIndex;
              const IconComp = item.icon;
              return (
                <button
                  key={item.id}
                  onClick={item.action}
                  onMouseEnter={() => setSelectedIndex(idx)}
                  className={`w-full flex items-center justify-between px-3 py-2.5 rounded-lg text-left text-xs transition-colors group ${
                    isSelected
                      ? "bg-emerald-500/10 border border-emerald-500/30 text-white"
                      : "text-slate-300 hover:bg-white/5 border border-transparent"
                  }`}
                >
                  <div className="flex items-center gap-3 min-w-0">
                    <div
                      className={`w-7 h-7 rounded-md flex items-center justify-center shrink-0 ${
                        isSelected
                          ? "bg-emerald-500/20 text-emerald-400"
                          : "bg-white/5 text-slate-400 group-hover:text-slate-200"
                      }`}
                    >
                      <IconComp className="w-4 h-4" />
                    </div>
                    <div className="min-w-0 truncate">
                      <div className="font-semibold truncate flex items-center gap-2">
                        <span>{item.title}</span>
                        {item.badge && (
                          <span
                            className={`text-[9px] font-mono px-1.5 py-0.2 rounded border ${
                              item.badge === "SETTLED"
                                ? "bg-emerald-500/10 text-emerald-300 border-emerald-500/25"
                                : item.badge === "UNMATCHED"
                                ? "bg-amber-500/10 text-amber-300 border-amber-500/25"
                                : "bg-white/5 text-slate-400 border-white/10"
                            }`}
                          >
                            {item.badge}
                          </span>
                        )}
                      </div>
                      {item.subtitle && (
                        <div className="text-[10px] text-slate-400 font-mono truncate mt-0.5">
                          {item.subtitle}
                        </div>
                      )}
                    </div>
                  </div>

                  <div className="flex items-center gap-2 shrink-0 pl-2">
                    <span className="text-[9px] font-mono text-slate-500 uppercase tracking-wider">
                      {item.category}
                    </span>
                    <ArrowRight
                      className={`w-3.5 h-3.5 transition-transform ${
                        isSelected
                          ? "text-emerald-400 translate-x-0.5"
                          : "text-slate-600 opacity-0 group-hover:opacity-100"
                      }`}
                    />
                  </div>
                </button>
              );
            })
          )}
        </div>

        {/* Footer shortcuts helper */}
        <div className="px-4 py-2 border-t border-white/5 bg-obsidian-900/80 flex items-center justify-between text-[10px] font-mono text-slate-500 shrink-0">
          <div className="flex items-center gap-4">
            <span className="flex items-center gap-1">
              <kbd className="px-1 py-0.2 rounded bg-white/5 border border-white/10">↑</kbd>
              <kbd className="px-1 py-0.2 rounded bg-white/5 border border-white/10">↓</kbd>
              Navigate
            </span>
            <span className="flex items-center gap-1">
              <kbd className="px-1.5 py-0.2 rounded bg-white/5 border border-white/10">↵</kbd>
              Select
            </span>
          </div>
          <span className="text-slate-400">
            {filteredItems.length} {filteredItems.length === 1 ? "result" : "results"}
          </span>
        </div>
      </div>
    </div>
  );
}

export default CommandPalette;
