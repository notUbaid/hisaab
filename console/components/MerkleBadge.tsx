"use client";

import React, { useState } from "react";
import { ShieldCheck, GitCommit, CheckCircle2, Copy, Check, X, Lock, AlertTriangle, RotateCw } from "lucide-react";
import { apiClient } from "@/lib/apiClient";

interface MerkleBadgeProps {
  rootHash: string;
  leavesCount: number;
  verified?: boolean;
  signatureEd25519?: string;
  batchId?: string;
}

export function MerkleBadge({
  rootHash,
  leavesCount,
  signatureEd25519,
  batchId = "BATCH-20261030-01",
}: MerkleBadgeProps) {
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [isVerifying, setIsVerifying] = useState(false);
  const [verificationStatus, setVerificationStatus] = useState<"UNVERIFIED" | "VERIFIED" | "FAILED">("UNVERIFIED");
  const [verificationDetails, setVerificationDetails] = useState<string | null>(null);
  const [isCopied, setIsCopied] = useState(false);

  // Authentically verify the RFC 6962 Merkle proof chain by calling the server-side audit verifier
  const handleVerifyProof = async () => {
    setIsVerifying(true);
    setVerificationDetails(null);

    try {
      const cleanRoot = rootHash.replace(/^sha256:/, "");
      const res = await apiClient.post<{
        verified: boolean;
        inclusionVerified: boolean;
        rootMatchesClaimed: boolean;
        rootHash: string;
        claimedRoot: string;
        invariantsPassed: number;
        totalInvariants: number;
        auditPath: { hash: string; direction: "left" | "right" }[];
        error?: string;
      }>("/api/audit/verify", {
        batchId,
        claimedRoot: cleanRoot,
        leafIndex: 0,
      });

      if (res.inclusionVerified) {
        setVerificationStatus("VERIFIED");
        setVerificationDetails(
          `RFC 6962 inclusion proof verified: Leaf 0 (${res.auditPath.length} sibling hashes) correctly ties to root ${res.rootHash.slice(0, 12)}...`
        );
      } else {
        setVerificationStatus("FAILED");
        setVerificationDetails(
          `Cryptographic Verification Failed: Reconstructed root does not match claimed batch root.`
        );
      }
    } catch (err: any) {
      setVerificationStatus("FAILED");
      setVerificationDetails(`Verification Error: ${err.message || "Audit engine unavailable"}`);
    } finally {
      setIsVerifying(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setIsCopied(true);
    setTimeout(() => setIsCopied(false), 2000);
  };

  const cleanHash = rootHash ? rootHash.replace(/^sha256:/, "") : "";
  const shortHash = cleanHash
    ? `${cleanHash.slice(0, 8)}...${cleanHash.slice(-8)}`
    : "96657a38...03777cb";

  return (
    <>
      {/* ── Visual Badge / Pill Trigger ── */}
      <div
        onClick={() => setIsModalOpen(true)}
        className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md bg-obsidian-900 border ${
          verificationStatus === "VERIFIED"
            ? "border-emerald-500/40 text-emerald-400"
            : verificationStatus === "FAILED"
            ? "border-rose-500/40 text-rose-400"
            : "border-white/10 text-slate-300 hover:border-emerald-500/30"
        } font-mono text-xs cursor-pointer transition select-none group`}
        title="Click to view & verify RFC 6962 Cryptographic Inclusion Proof"
      >
        <ShieldCheck
          className={`w-3.5 h-3.5 ${
            verificationStatus === "VERIFIED"
              ? "text-emerald-400"
              : verificationStatus === "FAILED"
              ? "text-rose-400"
              : "text-slate-400 group-hover:text-emerald-400"
          } group-hover:scale-110 transition-transform`}
        />
        <span className="font-semibold text-white">Merkle:</span>
        <span className="text-slate-300 font-mono">{shortHash}</span>
        <span
          className={`text-[10px] px-1 py-0.2 rounded border ${
            verificationStatus === "VERIFIED"
              ? "bg-emerald-500/10 border-emerald-500/20 text-emerald-400"
              : verificationStatus === "FAILED"
              ? "bg-rose-500/10 border-rose-500/20 text-rose-400"
              : "bg-white/5 border-white/10 text-slate-400"
          }`}
        >
          {verificationStatus === "VERIFIED" ? "Verified ✓" : `${leavesCount} leaves`}
        </span>
      </div>

      {/* ── Interactive Cryptographic Hash Tree Modal ── */}
      {isModalOpen && (
        <div
          className="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4 font-sans animate-in fade-in duration-150"
          onClick={() => setIsModalOpen(false)}
        >
          <div
            className="w-full max-w-2xl bg-obsidian-850 border border-white/10 rounded-xl shadow-2xl overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Modal Header */}
            <div className="p-4 border-b border-white/10 flex items-center justify-between bg-obsidian-900/80">
              <div className="flex items-center gap-2.5">
                <div
                  className={`w-8 h-8 rounded-lg ${
                    verificationStatus === "VERIFIED"
                      ? "bg-emerald-500/10 border-emerald-500/30 text-emerald-400"
                      : verificationStatus === "FAILED"
                      ? "bg-rose-500/10 border-rose-500/30 text-rose-400"
                      : "bg-obsidian-900 border-white/10 text-slate-400"
                  } border flex items-center justify-center`}
                >
                  <Lock className="w-4 h-4" />
                </div>
                <div>
                  <h3 className="font-bold text-sm text-white flex items-center gap-2">
                    RFC 6962 Merkle Tree Audit Proof
                    {verificationStatus === "VERIFIED" && (
                      <span className="pill-emerald text-[9px]">Verified</span>
                    )}
                    {verificationStatus === "FAILED" && (
                      <span className="pill-rose text-[9px]">Verification Failed</span>
                    )}
                    {verificationStatus === "UNVERIFIED" && (
                      <span className="pill-amber text-[9px]">Unverified</span>
                    )}
                  </h3>
                  <div className="text-[11px] font-mono text-slate-400 mt-0.5">
                    Batch: <span className="text-slate-200">{batchId}</span> · Domain Separation: 0x00 (leaf) / 0x01 (node)
                  </div>
                </div>
              </div>

              <button
                onClick={() => setIsModalOpen(false)}
                className="p-1 rounded-lg text-slate-400 hover:text-white hover:bg-white/5"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {/* Modal Body: Interactive Tree Diagram */}
            <div className="p-5 space-y-5 text-xs font-mono">
              {/* Root Node */}
              <div
                className={`p-3.5 rounded-lg bg-obsidian-900 border ${
                  verificationStatus === "VERIFIED"
                    ? "border-emerald-500/40"
                    : verificationStatus === "FAILED"
                    ? "border-rose-500/40"
                    : "border-white/10"
                } space-y-2`}
              >
                <div className="flex items-center justify-between text-[10px] text-slate-400 uppercase tracking-wider">
                  <span
                    className={`font-bold flex items-center gap-1 ${
                      verificationStatus === "VERIFIED"
                        ? "text-emerald-400"
                        : verificationStatus === "FAILED"
                        ? "text-rose-400"
                        : "text-slate-300"
                    }`}
                  >
                    <ShieldCheck className="w-3.5 h-3.5" />
                    Merkle Root (Level 2)
                  </span>
                  <button
                    onClick={() => copyToClipboard(cleanHash)}
                    className="flex items-center gap-1 text-slate-400 hover:text-white"
                  >
                    {isCopied ? <Check className="w-3 h-3 text-emerald-400" /> : <Copy className="w-3 h-3" />}
                    <span>{isCopied ? "Copied" : "Copy Root"}</span>
                  </button>
                </div>
                <div className="text-xs text-white break-all bg-obsidian-950 p-2 rounded border border-white/5 font-semibold">
                  {cleanHash}
                </div>
              </div>

              {/* Connecting Tree Lines */}
              <div className="flex justify-center text-slate-600">
                <span>│ ─────────────── ┴ ─────────────── │</span>
              </div>

              {/* Intermediate Level Nodes */}
              <div className="grid grid-cols-2 gap-4">
                <div className="p-3 rounded-lg bg-obsidian-900 border border-white/10 space-y-1.5">
                  <div className="text-[10px] text-cyan-400 uppercase font-semibold">
                    Node L1-A (Hash 0+1)
                  </div>
                  <div className="text-[11px] text-slate-300 truncate">
                    sha256(0x01 || leaf0 || leaf1)
                  </div>
                </div>

                <div className="p-3 rounded-lg bg-obsidian-900 border border-white/10 space-y-1.5">
                  <div className="text-[10px] text-purple-400 uppercase font-semibold">
                    Node L1-B (Hash 2+3)
                  </div>
                  <div className="text-[11px] text-slate-300 truncate">
                    sha256(0x01 || leaf2 || leaf3)
                  </div>
                </div>
              </div>

              {/* Connecting Tree Lines */}
              <div className="flex justify-around text-slate-600 px-8">
                <span>┌────┴────┐</span>
                <span>┌────┴────┐</span>
              </div>

              {/* Leaf Nodes */}
              <div className="grid grid-cols-4 gap-2 text-[10px]">
                <div className="p-2 rounded bg-obsidian-950 border border-white/5 space-y-1">
                  <span className="text-emerald-400 font-bold">Leaf 0 [Entry 0]</span>
                  <div className="text-slate-400 truncate">collector debit</div>
                </div>
                <div className="p-2 rounded bg-obsidian-950 border border-white/5 space-y-1">
                  <span className="text-emerald-400 font-bold">Leaf 1 [Entry 1]</span>
                  <div className="text-slate-400 truncate">receiver credit</div>
                </div>
                <div className="p-2 rounded bg-obsidian-950 border border-white/5 space-y-1">
                  <span className="text-emerald-400 font-bold">Leaf 2 [Entry 2]</span>
                  <div className="text-slate-400 truncate">tax:tds credit</div>
                </div>
                <div className="p-2 rounded bg-obsidian-950 border border-white/5 space-y-1">
                  <span className="text-emerald-400 font-bold">Leaf 3 [Entry 3]</span>
                  <div className="text-slate-400 truncate">tax:gst_tcs credit</div>
                </div>
              </div>

              {/* Verification Details Alert */}
              {verificationDetails && (
                <div
                  className={`p-3 rounded-lg border text-xs font-mono ${
                    verificationStatus === "VERIFIED"
                      ? "bg-emerald-500/10 border-emerald-500/30 text-emerald-300"
                      : "bg-rose-500/10 border-rose-500/30 text-rose-300"
                  }`}
                >
                  <div className="flex items-start gap-2">
                    {verificationStatus === "VERIFIED" ? (
                      <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0 mt-0.5" />
                    ) : (
                      <AlertTriangle className="w-4 h-4 text-rose-400 shrink-0 mt-0.5" />
                    )}
                    <span>{verificationDetails}</span>
                  </div>
                </div>
              )}

              {/* Ed25519 Network Signature Info */}
              {signatureEd25519 && (
                <div className="pt-3 border-t border-white/10 space-y-1">
                  <div className="text-[10px] uppercase text-slate-500">
                    Network Ed25519 Signature (RFC 8032):
                  </div>
                  <div className="text-[10px] text-slate-400 break-all bg-obsidian-950 p-2 rounded border border-white/5">
                    {signatureEd25519}
                  </div>
                </div>
              )}
            </div>

            {/* Modal Footer Controls */}
            <div className="p-4 border-t border-white/10 bg-obsidian-900/90 flex items-center justify-between">
              <div className="text-[11px] font-mono text-slate-400">
                {verificationStatus === "VERIFIED" ? (
                  <span className="text-emerald-400 font-semibold flex items-center gap-1.5">
                    <CheckCircle2 className="w-4 h-4" /> Inclusion path verified against root
                  </span>
                ) : verificationStatus === "FAILED" ? (
                  <span className="text-rose-400 font-semibold flex items-center gap-1.5">
                    <AlertTriangle className="w-4 h-4" /> Proof verification failed
                  </span>
                ) : (
                  <span>Ready to verify proof chain</span>
                )}
              </div>

              <button
                onClick={handleVerifyProof}
                disabled={isVerifying}
                className="py-1.5 px-4 rounded-lg bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white font-semibold text-xs transition flex items-center gap-2 shadow-glow"
              >
                {isVerifying ? (
                  <>
                    <RotateCw className="w-3.5 h-3.5 animate-spin" />
                    <span>Verifying Proof...</span>
                  </>
                ) : (
                  <>
                    <ShieldCheck className="w-3.5 h-3.5" />
                    <span>Verify Proof Chain</span>
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

export default MerkleBadge;
