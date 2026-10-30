"use client";

import React, { useState, useMemo } from "react";
import { Receipt, Info, Sparkles, Scale } from "lucide-react";
import MoneyCell, { formatIndianNumbering } from "@/components/MoneyCell";

interface TaxWaterfallProps {
  initialOrderValuePaise?: number;
  initialBffBps?: number;
  initialCommissionBps?: number;
  editable?: boolean;
}

export function TaxWaterfall({
  initialOrderValuePaise = 100000, // ₹1,000.00
  initialBffBps = 300, // 3.00%
  initialCommissionBps = 500, // 5.00%
  editable = true,
}: TaxWaterfallProps) {
  const [orderValuePaise, setOrderValuePaise] = useState<number>(initialOrderValuePaise);
  const [bffBps, setBffBps] = useState<number>(initialBffBps);
  const [commissionBps, setCommissionBps] = useState<number>(initialCommissionBps);
  const [hasNoPAN, setHasNoPAN] = useState<boolean>(false);

  // Banker's Rounding helper (RoundHalfEven)
  const roundHalfEven = (num: number, denom: number): number => {
    if (denom <= 0) return 0;
    const quot = Math.floor(num / denom);
    const rem = num % denom;
    const half = denom / 2;
    if (rem > half) return quot + 1;
    if (rem < half) return quot;
    return quot % 2 !== 0 ? quot + 1 : quot;
  };

  // Tax calculations per internal/nptoolkit/tax.go
  const calculations = useMemo(() => {
    const gross = Math.max(0, orderValuePaise);
    const bff = roundHalfEven(gross * bffBps, 10000);
    const commission = roundHalfEven(gross * commissionBps, 10000);

    // Statutory Withholding rates:
    // TDS Sec 194-O: 10 bps (0.10%), or 500 bps (5.00%) if HasNoPAN
    const tdsRateBps = hasNoPAN ? 500 : 10;
    const tds = roundHalfEven(gross * tdsRateBps, 10000);

    // GST TCS Sec 52: 50 bps (0.50%) per CGST Notification 15/2024
    const tcsRateBps = 50;
    const tcs = roundHalfEven(gross * tcsRateBps, 10000);

    const totalTax = tds + tcs;
    const totalDeductions = bff + commission + totalTax;
    const netPayout = Math.max(0, gross - totalDeductions);

    // Percentages for the stacked bar
    const netPct = gross > 0 ? (netPayout / gross) * 100 : 0;
    const bffPct = gross > 0 ? (bff / gross) * 100 : 0;
    const commPct = gross > 0 ? (commission / gross) * 100 : 0;
    const tdsPct = gross > 0 ? (tds / gross) * 100 : 0;
    const tcsPct = gross > 0 ? (tcs / gross) * 100 : 0;

    return {
      gross,
      bff,
      commission,
      tds,
      tcs,
      totalTax,
      totalDeductions,
      netPayout,
      netPct,
      bffPct,
      commPct,
      tdsPct,
      tcsPct,
      tdsRateBps,
    };
  }, [orderValuePaise, bffBps, commissionBps, hasNoPAN]);

  return (
    <div className="fin-card overflow-hidden">
      {/* ── Card Header ── */}
      <div className="px-4 py-3 bg-obsidian-900/90 border-b border-white/5 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Receipt className="w-4 h-4 text-emerald-400" />
          <span className="font-semibold text-xs text-white uppercase tracking-wider">
            Statutory Tax Waterfall (BRD §4 & CGST Sec 52)
          </span>
        </div>
        <div className="flex items-center gap-1.5 text-[10px] font-mono text-slate-400">
          <Scale className="w-3.5 h-3.5 text-emerald-400" />
          <span>Banker&apos;s RoundHalfEven</span>
        </div>
      </div>

      <div className="p-4 space-y-4 text-xs font-sans">
        {/* ── Interactive Input Sliders (if editable) ── */}
        {editable && (
          <div className="grid grid-cols-3 gap-3 p-3 rounded-lg bg-obsidian-900 border border-white/5 font-mono text-[11px]">
            <div>
              <div className="flex justify-between items-center text-slate-400 mb-1">
                <span>Order Value:</span>
                <span className="text-white font-bold">₹{(orderValuePaise / 100).toFixed(2)}</span>
              </div>
              <input
                type="range"
                min="10000"
                max="5000000"
                step="5000"
                value={orderValuePaise}
                onChange={(e) => setOrderValuePaise(Number(e.target.value))}
                className="w-full h-1 bg-white/10 rounded-lg appearance-none cursor-pointer accent-emerald-500"
              />
            </div>

            <div>
              <div className="flex justify-between items-center text-slate-400 mb-1">
                <span>BFF Fee:</span>
                <span className="text-white font-bold">{(bffBps / 100).toFixed(2)}%</span>
              </div>
              <input
                type="range"
                min="0"
                max="1000"
                step="25"
                value={bffBps}
                onChange={(e) => setBffBps(Number(e.target.value))}
                className="w-full h-1 bg-white/10 rounded-lg appearance-none cursor-pointer accent-emerald-500"
              />
            </div>

            <div>
              <div className="flex justify-between items-center text-slate-400 mb-1">
                <span>Commission:</span>
                <span className="text-white font-bold">{(commissionBps / 100).toFixed(2)}%</span>
              </div>
              <input
                type="range"
                min="0"
                max="1500"
                step="50"
                value={commissionBps}
                onChange={(e) => setCommissionBps(Number(e.target.value))}
                className="w-full h-1 bg-white/10 rounded-lg appearance-none cursor-pointer accent-emerald-500"
              />
            </div>
          </div>
        )}

        {/* ── Proportional Stacked Bar ── */}
        <div>
          <div className="flex justify-between items-center text-[10px] font-mono text-slate-400 mb-1.5">
            <span>Gross Order: {formatIndianNumbering(calculations.gross).full}</span>
            <div className="flex items-center gap-2">
              <label className="flex items-center gap-1 cursor-pointer text-[10px]">
                <input
                  type="checkbox"
                  checked={hasNoPAN}
                  onChange={(e) => setHasNoPAN(e.target.checked)}
                  className="rounded bg-obsidian-900 border-white/20 text-emerald-500 text-xs"
                />
                <span className={hasNoPAN ? "text-rose-400 font-semibold" : "text-slate-400"}>
                  No PAN (5% TDS)
                </span>
              </label>
            </div>
          </div>

          <div className="h-4 w-full rounded-md overflow-hidden flex bg-obsidian-900 border border-white/5">
            {/* Net Payout */}
            <div
              style={{ width: `${calculations.netPct}%` }}
              className="bg-emerald-500 hover:bg-emerald-400 transition-all cursor-pointer relative group"
              title={`Net Seller Payout: ₹${(calculations.netPayout / 100).toFixed(2)} (${calculations.netPct.toFixed(1)}%)`}
            />
            {/* BFF */}
            <div
              style={{ width: `${calculations.bffPct}%` }}
              className="bg-cyan-500 hover:bg-cyan-400 transition-all cursor-pointer"
              title={`Buyer Finder Fee: ₹${(calculations.bff / 100).toFixed(2)} (${calculations.bffPct.toFixed(1)}%)`}
            />
            {/* Commission */}
            <div
              style={{ width: `${calculations.commPct}%` }}
              className="bg-purple-500 hover:bg-purple-400 transition-all cursor-pointer"
              title={`Platform Commission: ₹${(calculations.commission / 100).toFixed(2)} (${calculations.commPct.toFixed(1)}%)`}
            />
            {/* TDS */}
            <div
              style={{ width: `${calculations.tdsPct}%` }}
              className="bg-amber-500 hover:bg-amber-400 transition-all cursor-pointer"
              title={`TDS Sec 194-O: ₹${(calculations.tds / 100).toFixed(2)} (${calculations.tdsPct.toFixed(1)}%)`}
            />
            {/* TCS */}
            <div
              style={{ width: `${calculations.tcsPct}%` }}
              className="bg-rose-500 hover:bg-rose-400 transition-all cursor-pointer"
              title={`GST TCS Sec 52: ₹${(calculations.tcs / 100).toFixed(2)} (${calculations.tcsPct.toFixed(1)}%)`}
            />
          </div>

          {/* Legend */}
          <div className="flex flex-wrap items-center gap-3 mt-2 text-[10px] font-mono text-slate-400">
            <span className="flex items-center gap-1">
              <span className="w-2 h-2 rounded-full bg-emerald-500"></span> Net Payout ({calculations.netPct.toFixed(1)}%)
            </span>
            <span className="flex items-center gap-1">
              <span className="w-2 h-2 rounded-full bg-cyan-500"></span> BFF ({calculations.bffPct.toFixed(1)}%)
            </span>
            <span className="flex items-center gap-1">
              <span className="w-2 h-2 rounded-full bg-purple-500"></span> Comm ({calculations.commPct.toFixed(1)}%)
            </span>
            <span className="flex items-center gap-1">
              <span className="w-2 h-2 rounded-full bg-amber-500"></span> TDS ({calculations.tdsPct.toFixed(2)}%)
            </span>
            <span className="flex items-center gap-1">
              <span className="w-2 h-2 rounded-full bg-rose-500"></span> GST TCS ({calculations.tcsPct.toFixed(2)}%)
            </span>
          </div>
        </div>

        {/* ── Paise-Exact Breakdown Grid ── */}
        <div className="grid grid-cols-2 gap-3 pt-3 border-t border-white/5 font-mono text-xs">
          <div className="space-y-1.5 p-3 rounded-lg bg-obsidian-900 border border-white/5">
            <div className="text-[10px] text-slate-500 uppercase tracking-wider">Statutory Deductions</div>
            <div className="flex justify-between items-center text-amber-400">
              <span>(-) TDS Sec 194-O ({(calculations.tdsRateBps / 100).toFixed(2)}%):</span>
              <MoneyCell paise={calculations.tds} direction="debit" showSign />
            </div>
            <div className="flex justify-between items-center text-rose-400">
              <span>(-) GST TCS Sec 52 (0.50%):</span>
              <MoneyCell paise={calculations.tcs} direction="debit" showSign />
            </div>
            <div className="pt-1 border-t border-white/5 flex justify-between items-center text-slate-300 font-semibold text-[11px]">
              <span>Total Tax Withholding:</span>
              <MoneyCell paise={calculations.totalTax} direction="debit" />
            </div>
          </div>

          <div className="space-y-1.5 p-3 rounded-lg bg-obsidian-900 border border-white/5">
            <div className="text-[10px] text-slate-500 uppercase tracking-wider">Net Settlement Wire</div>
            <div className="flex justify-between items-center text-cyan-400">
              <span>(-) Buyer Finder Fee:</span>
              <MoneyCell paise={calculations.bff} direction="debit" showSign />
            </div>
            <div className="flex justify-between items-center text-purple-400">
              <span>(-) Platform Commission:</span>
              <MoneyCell paise={calculations.commission} direction="debit" showSign />
            </div>
            <div className="pt-1 border-t border-white/5 flex justify-between items-center text-emerald-400 font-bold text-sm">
              <span>Net Wire Payout:</span>
              <MoneyCell paise={calculations.netPayout} />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

export default TaxWaterfall;
