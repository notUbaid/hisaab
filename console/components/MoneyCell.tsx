import React from "react";

interface MoneyCellProps {
  paise: number | bigint;
  direction?: "debit" | "credit" | "net" | "neutral";
  showSign?: boolean;
  currencyPrefix?: boolean;
  className?: string;
}

export function formatPaiseToINR(paise: number | bigint): string {
  const num = typeof paise === "bigint" ? Number(paise) : paise;
  const inr = Math.abs(num / 100).toLocaleString("en-IN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return inr;
}

export function MoneyCell({
  paise,
  direction = "neutral",
  showSign = false,
  currencyPrefix = true,
  className = "",
}: MoneyCellProps) {
  const num = typeof paise === "bigint" ? Number(paise) : paise;
  const isPositive = num > 0;
  const isNegative = num < 0;

  let colorClass = "text-zinc-200";
  let sign = "";

  if (direction === "credit") {
    colorClass = "text-emerald-400";
    if (showSign) sign = "+";
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
    }
  }

  const inrString = formatPaiseToINR(num);

  return (
    <span
      className={`font-mono tabular-nums inline-block text-right ${colorClass} ${className}`}
      title={`${num} paise`}
    >
      {sign}
      {currencyPrefix ? "₹" : ""}
      {inrString}
    </span>
  );
}

export default MoneyCell;
