"use client";

import React, { useState } from "react";
import { ShieldCheck, ChevronDown, ChevronUp, Key, Clock, CheckCircle2, AlertTriangle, FileCode } from "lucide-react";

export interface SignatureInspectorProps {
  authHeader?: string;
  subscriberId?: string;
  bapId?: string;
  bppId?: string;
  bodyDigest?: string;
  createdAtEpoch?: number;
  expiresAtEpoch?: number;
  createdTimestamp?: string;
  expiresTimestamp?: string;
  isVerified?: boolean;
  className?: string;
}

export function SignatureInspector({
  authHeader,
  subscriberId,
  bapId,
  bppId,
  bodyDigest = "BLAKE-512=YTI5OGQ4ZjE5MGEwNGU3OGRhMzU3MzljYjRhNTM2Zm...",
  createdAtEpoch,
  expiresAtEpoch,
  createdTimestamp,
  expiresTimestamp,
  isVerified = true,
  className = "",
}: SignatureInspectorProps) {
  const [isOpen, setIsOpen] = useState(false);

  const activeSubscriber = subscriberId || bapId || "buyer-app.ondc.org";

  // Parse timestamps if provided as ISO strings
  const parsedCreatedEpoch = createdTimestamp
    ? Math.floor(new Date(createdTimestamp).getTime() / 1000)
    : createdAtEpoch;
  const parsedExpiresEpoch = expiresTimestamp
    ? Math.floor(new Date(expiresTimestamp).getTime() / 1000)
    : expiresAtEpoch;

  // Default synthetic Beckn Authorization header if not provided
  const nowSec = Math.floor(Date.now() / 1000);
  const created = parsedCreatedEpoch || nowSec - 25;
  const expires = parsedExpiresEpoch || nowSec + 275; // 300s standard TTL

  const defaultHeader =
    authHeader ||
    `Signature keyId="${activeSubscriber}|key-2026-1|ed25519",algorithm="ed25519",created="${created}",expires="${expires}",headers="(created) (expires) digest",signature="${
      isVerified
        ? "sK7P+wY3r7bZ6k+9vH3+Q1aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789abcdefg=="
        : "TAMPERED_BYTE_INVALID_BLAKE512_DIGEST_SIG_FAIL=="
    }"`;

  // Parse header parameters
  const keyIdMatch = defaultHeader.match(/keyId="([^"]+)"/);
  const algoMatch = defaultHeader.match(/algorithm="([^"]+)"/);
  const sigMatch = defaultHeader.match(/signature="([^"]+)"/);

  const keyId = keyIdMatch ? keyIdMatch[1] : `${activeSubscriber}|key-1|ed25519`;
  const algorithm = algoMatch ? algoMatch[1] : "ed25519";
  const signature = sigMatch ? sigMatch[1] : "sK7P+wY3r7bZ6k+9vH3+Q1aBcDe...";

  // Clock Skew & TTL calculation
  const totalWindowSec = expires - created;
  const elapsedSec = Math.max(0, nowSec - created);
  const remainingSec = Math.max(0, expires - nowSec);
  const isExpired = nowSec > expires;
  const isClockSkewSafe = elapsedSec >= 0 && elapsedSec <= 300;

  return (
    <div className={`rounded-lg border border-white/10 bg-obsidian-900 text-xs font-mono ${className}`}>
      {/* Header bar */}
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="w-full px-3 py-2 flex items-center justify-between hover:bg-white/5 transition-colors select-none"
      >
        <div className="flex items-center gap-2">
          {isVerified ? (
            <ShieldCheck className="w-4 h-4 text-emerald-400" />
          ) : (
            <AlertTriangle className="w-4 h-4 text-rose-400" />
          )}
          <span className="font-semibold text-white">Beckn Ed25519 Auth Signature</span>
          <span className={isVerified ? "pill-emerald text-[10px]" : "pill-rose text-[10px]"}>
            {isVerified ? "Verified" : "Tampered (Digest Mismatch)"}
          </span>
        </div>

        <div className="flex items-center gap-2 text-slate-400">
          <span className="text-[10px] text-slate-500">{activeSubscriber}</span>
          {isOpen ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
        </div>
      </button>

      {/* Expanded details */}
      {isOpen && (
        <div className="p-3 border-t border-white/5 space-y-3 bg-obsidian-850">
          {/* Key ID & Digest */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-[11px]">
            <div className="p-2 rounded bg-obsidian-900 border border-white/5 space-y-1">
              <span className="text-slate-500 uppercase text-[9px] flex items-center gap-1">
                <Key className="w-2.5 h-2.5 text-slate-400" />
                Subscriber Key ID
              </span>
              <p className="text-slate-200 truncate">{keyId}</p>
            </div>

            <div className="p-2 rounded bg-obsidian-900 border border-white/5 space-y-1">
              <span className="text-slate-500 uppercase text-[9px] flex items-center gap-1">
                <FileCode className="w-2.5 h-2.5 text-slate-400" />
                Digest Algorithm (ONDC BLAKE-512)
              </span>
              <p className="text-emerald-300 truncate">{bodyDigest.slice(0, 32)}...</p>
            </div>
          </div>

          {/* Clock-Skew & TTL Gauge */}
          <div className="p-2 rounded bg-obsidian-900 border border-white/5 space-y-2">
            <div className="flex items-center justify-between text-[10px]">
              <span className="text-slate-400 flex items-center gap-1">
                <Clock className="w-3 h-3 text-amber-400" />
                Signature TTL Window ({totalWindowSec}s standard)
              </span>
              <span className={isExpired ? "text-rose-400" : "text-emerald-400 font-bold"}>
                {isExpired ? "EXPIRED" : `${remainingSec}s remaining`}
              </span>
            </div>

            {/* Gauge progress bar */}
            <div className="h-1.5 w-full bg-obsidian-950 rounded-full overflow-hidden">
              <div
                className={`h-full transition-all duration-500 ${
                  isExpired ? "bg-rose-500" : "bg-emerald-500"
                }`}
                style={{ width: `${Math.min(100, (elapsedSec / Math.max(1, totalWindowSec)) * 100)}%` }}
              />
            </div>

            <div className="flex items-center justify-between text-[9px] text-slate-500">
              <span>Created: {new Date(created * 1000).toLocaleTimeString()}</span>
              <span>Expires: {new Date(expires * 1000).toLocaleTimeString()}</span>
            </div>
          </div>

          {/* Raw Signature Box */}
          <div className="p-2 rounded bg-obsidian-950 border border-white/5 font-mono text-[10px] text-slate-400 break-all space-y-1">
            <span className="text-slate-500 uppercase text-[9px] block">Ed25519 Base64 Signature (64-byte payload)</span>
            <span className="text-slate-300">{signature}</span>
          </div>

          {/* Compliance Status Checks */}
          <div className="flex flex-wrap items-center gap-3 text-[10px] text-slate-300 pt-1 border-t border-white/5">
            <div className="flex items-center gap-1">
              <CheckCircle2 className="w-3 h-3 text-emerald-400" />
              <span>BLAKE-512 Digest Match</span>
            </div>
            <div className="flex items-center gap-1">
              <CheckCircle2 className="w-3 h-3 text-emerald-400" />
              <span>Clock-Skew &lt; 300s</span>
            </div>
            <div className="flex items-center gap-1">
              <CheckCircle2 className="w-3 h-3 text-emerald-400" />
              <span>ONDC Registry Key Valid</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
