"use client";

import React, { useState, useEffect } from "react";
import { Eye, EyeOff, ShieldCheck, Lock, AlertCircle } from "lucide-react";
import { ParticipantPerspective } from "@/lib/rbac";
import { apiClient } from "@/lib/apiClient";

interface EncryptedFieldProps {
  value: string;
  label?: string;
  fieldType?: "account_number" | "ifsc" | "vpa" | "utr";
  className?: string;
}

export function EncryptedField({
  value,
  label,
  fieldType = "account_number",
  className = "",
}: EncryptedFieldProps) {
  const [isRevealed, setIsRevealed] = useState(false);
  const [secondsRemaining, setSecondsRemaining] = useState(15);
  const [rbacError, setRbacError] = useState<string | null>(null);

  // Derive mask: Show only last 4 chars, mask rest with bullets
  const maskedValue = React.useMemo(() => {
    if (!value) return "••••";
    if (value.length <= 4) return "••••";
    const visibleLength = fieldType === "ifsc" ? 4 : 4;
    const maskedPart = "•".repeat(Math.max(4, value.length - visibleLength));
    const visiblePart = value.slice(-visibleLength);
    return `${maskedPart} ${visiblePart}`;
  }, [value, fieldType]);

  // 15-second auto-relock countdown timer
  useEffect(() => {
    let interval: NodeJS.Timeout | null = null;
    if (isRevealed) {
      interval = setInterval(() => {
        setSecondsRemaining((prev) => {
          if (prev <= 1) {
            setIsRevealed(false);
            return 15;
          }
          return prev - 1;
        });
      }, 1000);
    } else {
      setSecondsRemaining(15);
    }

    return () => {
      if (interval) clearInterval(interval);
    };
  }, [isRevealed]);

  const handleToggleReveal = (e: React.MouseEvent) => {
    e.stopPropagation();

    if (isRevealed) {
      setIsRevealed(false);
      setSecondsRemaining(15);
      return;
    }

    const currentRole = apiClient.getRole();
    // RBAC check: Only Collector RSP can decrypt counterparty raw bank accounts
    if (currentRole !== "COLLECTOR_RSP") {
      setRbacError(`Access Denied: ${currentRole} cannot decrypt PII identifiers`);
      setTimeout(() => setRbacError(null), 3500);
      return;
    }

    setIsRevealed(true);
    setSecondsRemaining(15);

    // Record audit trail event for security compliance
    if (typeof window !== "undefined") {
      window.dispatchEvent(
        new CustomEvent("hisaab-audit-event", {
          detail: {
            action: "ENCRYPTED_FIELD_DECRYPT_REVEALED",
            field: fieldType,
            role: currentRole,
            timestamp: new Date().toISOString(),
          },
        })
      );
    }
  };

  return (
    <div className={`inline-flex flex-col gap-1 ${className}`}>
      {label && (
        <span className="text-[10px] font-mono text-slate-500 uppercase flex items-center gap-1">
          <Lock className="w-2.5 h-2.5 text-slate-500" />
          {label}
        </span>
      )}

      <div className="relative inline-flex items-center gap-2 group">
        <div
          onClick={handleToggleReveal}
          className={`font-mono text-xs px-2.5 py-1 rounded border transition-all cursor-pointer select-none flex items-center gap-2 ${
            isRevealed
              ? "bg-amber-500/10 border-amber-500/30 text-amber-200 shadow-glow"
              : "bg-obsidian-900 border-white/10 text-slate-300 hover:border-white/20"
          }`}
          title="Click to reveal encrypted identifier (requires RBAC approval)"
        >
          <span
            className={`transition-all duration-300 tracking-wider ${
              isRevealed ? "blur-0 text-white font-semibold" : "blur-[0.5px] text-slate-400"
            }`}
          >
            {isRevealed ? value : maskedValue}
          </span>

          {isRevealed ? (
            <div className="flex items-center gap-1.5 pl-1.5 border-l border-amber-500/30">
              <span className="text-[10px] font-mono font-bold text-amber-400">
                {secondsRemaining}s
              </span>
              <EyeOff className="w-3.5 h-3.5 text-amber-400 hover:text-amber-300" />
            </div>
          ) : (
            <Eye className="w-3.5 h-3.5 text-slate-500 group-hover:text-slate-300 transition-colors" />
          )}
        </div>

        {isRevealed && (
          <div className="absolute -bottom-1 left-0 right-0 h-0.5 bg-obsidian-900 rounded overflow-hidden">
            <div
              className="h-full bg-amber-400 transition-all duration-1000 ease-linear"
              style={{ width: `${(secondsRemaining / 15) * 100}%` }}
            />
          </div>
        )}
      </div>

      {rbacError && (
        <div className="text-[10px] font-mono text-rose-400 flex items-center gap-1 animate-in fade-in">
          <AlertCircle className="w-3 h-3 text-rose-400 shrink-0" />
          <span>{rbacError}</span>
        </div>
      )}
    </div>
  );
}
