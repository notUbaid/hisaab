"use client";

import React, { useState } from "react";

interface MoneyCellProps {
  paise: number | bigint;
  direction?: "debit" | "credit" | "net" | "neutral";
  showSign?: boolean;
  currencyPrefix?: boolean;
  className?: string;
  withPopover?: boolean;
}

/**
 * Format paise into Indian numbering format with comma grouping:
 * e.g., 12450000 paise -> ₹1,24,500.00
 */
export function formatIndianNumbering(paise: number | bigint): { whole: string; decimal: string; full: string } {
  const num = typeof paise === "bigint" ? Number(paise) : paise;
  const absNum = Math.abs(num);
  const rupees = Math.floor(absNum / 100);
  const remainingPaise = Math.floor(absNum % 100);

  // Indian number formatting for whole rupees:
  // Last 3 digits grouped, then groups of 2 digits
  const rupeeStr = rupees.toString();
  let formattedRupees = "";
  if (rupeeStr.length <= 3) {
    formattedRupees = rupeeStr;
  } else {
    const lastThree = rupeeStr.substring(rupeeStr.length - 3);
    const otherNumbers = rupeeStr.substring(0, rupeeStr.length - 3);
    const formattedOthers = otherNumbers.replace(/\B(?=(\d{2})+(?!\d))/g, ",");
    formattedRupees = formattedOthers + "," + lastThree;
  }

  const decimalStr = remainingPaise.toString().padStart(2, "0");
  return {
    whole: formattedRupees,
    decimal: decimalStr,
    full: `${num < 0 ? "-" : ""}₹${formattedRupees}.${decimalStr}`,
  };
}

export function formatPaiseToINR(paise: number | bigint): string {
  return formatIndianNumbering(paise).full;
}

export function MoneyCell({
  paise,
  direction = "neutral",
  showSign = false,
  currencyPrefix = true,
  className = "",
  withPopover = true,
}: MoneyCellProps) {
  const [showTooltip, setShowTooltip] = useState(false);
  const num = typeof paise === "bigint" ? Number(paise) : paise;
  const isPositive = num > 0;
  const isNegative = num < 0;

  let colorClass = "text-slate-200";
  let sign = "";

  if (direction === "credit") {
    colorClass = "text-emerald-400";
    if (showSign && isPositive) sign = "+";
  } else if (direction === "debit") {
    colorClass = "text-amber-400";
    if (showSign) sign = "-";
  } else if (direction === "net") {
    if (isPositive) {
      colorClass = "text-emerald-400";
      if (showSign) sign = "+";
    } else if (isNegative) {
      colorClass = "text-rose-400";
      if (showSign) sign = "-";
    } else {
      colorClass = "text-slate-400";
    }
  }

  const { whole, decimal } = formatIndianNumbering(num);

  // Calculate approximate statutory deductions for the popover (TDS 10 bps, GST TCS 50 bps)
  const tdsEstimatePaise = Math.round((Math.abs(num) * 10) / 10000);
  const tcsEstimatePaise = Math.round((Math.abs(num) * 50) / 10000);
  const totalTaxPaise = tdsEstimatePaise + tcsEstimatePaise;
  const netEstimatePaise = Math.max(0, Math.abs(num) - totalTaxPaise);

  return (
    <div
      className="relative inline-block"
      onMouseEnter={() => withPopover && setShowTooltip(true)}
      onMouseLeave={() => withPopover && setShowTooltip(false)}
    >
      <span
        className={`font-mono tabular-nums inline-flex items-baseline text-right cursor-default ${colorClass} ${className}`}
      >
        {sign && <span className="mr-0.5">{sign}</span>}
        {currencyPrefix && <span className="mr-0.5 text-[0.85em] opacity-80">₹</span>}
        <span className="font-medium">{whole}</span>
        <span className="text-[0.85em] opacity-50">.{decimal}</span>
      </span>

      {/* Institutional hover popover */}
      {withPopover && showTooltip && (
        <div className="absolute z-50 bottom-full left-1/2 -translate-x-1/2 mb-2 w-64 p-2.5 rounded-lg bg-obsidian-850 border border-white/10 shadow-2xl backdrop-blur-md text-left font-mono text-xs pointer-events-none animate-in fade-in zoom-in-95 duration-100">
          <div className="flex items-center justify-between pb-1.5 border-b border-white/10 text-[10px] text-slate-400 uppercase tracking-wider">
            <span>Money Breakdown</span>
            <span className="text-emerald-400 font-semibold">INR (₹)</span>
          </div>

          <div className="mt-2 space-y-1.5 text-[11px]">
            <div className="flex justify-between items-center text-slate-300">
              <span className="text-slate-400">Raw Integer:</span>
              <span className="text-white font-semibold">{num.toLocaleString()} paise</span>
            </div>
            <div className="flex justify-between items-center text-slate-300">
              <span className="text-slate-400">Standard Value:</span>
              <span className="text-white font-medium">₹{whole}.{decimal}</span>
            </div>
            <div className="pt-1.5 border-t border-white/5 space-y-1 text-[10px]">
              <div className="flex justify-between text-amber-400/90">
                <span>TDS 194-O (10 bps):</span>
                <span>₹{(tdsEstimatePaise / 100).toFixed(2)}</span>
              </div>
              <div className="flex justify-between text-amber-400/90">
                <span>GST TCS Sec 52 (50 bps):</span>
                <span>₹{(tcsEstimatePaise / 100).toFixed(2)}</span>
              </div>
              <div className="flex justify-between text-emerald-400 font-semibold pt-0.5 border-t border-white/5">
                <span>Est. Net Payout:</span>
                <span>₹{(netEstimatePaise / 100).toFixed(2)}</span>
              </div>
            </div>
          </div>
          <div className="absolute top-full left-1/2 -translate-x-1/2 -mt-1 border-4 border-transparent border-t-obsidian-850" />
        </div>
      )}
    </div>
  );
}

export default MoneyCell;
