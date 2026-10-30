// Participant Perspective & Financial Display Types for Hisaab Settlement Console

export type ParticipantPerspective = "COLLECTOR_RSP" | "BUYER_APP_BAP" | "SELLER_APP_BPP";

export interface PerspectiveConfig {
  id: ParticipantPerspective;
  name: string;
  roleDescription: string;
  badge: string;
}

export const PERSPECTIVES: Record<ParticipantPerspective, PerspectiveConfig> = {
  COLLECTOR_RSP: {
    id: "COLLECTOR_RSP",
    name: "Collector RSP (Nodal SA)",
    roleDescription: "Settlement Agency holding escrow/nodal funds & running netting reconciliation.",
    badge: "COLLECTOR_RSP",
  },
  BUYER_APP_BAP: {
    id: "BUYER_APP_BAP",
    name: "Buyer App (BAP)",
    roleDescription: "NP collecting customer payments, deducting buyer fees, submitting claim.",
    badge: "BUYER_APP_BAP",
  },
  SELLER_APP_BPP: {
    id: "SELLER_APP_BPP",
    name: "Seller App (BPP)",
    roleDescription: "NP fulfilling customer orders, receiving payout net of deductions.",
    badge: "SELLER_APP_BPP",
  },
};

export function formatPaiseToINR(paise: number | bigint): string {
  const num = typeof paise === "bigint" ? Number(paise) : paise;
  const inr = Math.abs(num / 100).toLocaleString("en-IN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return `${num < 0 ? "-" : ""}₹${inr}`;
}

export function maskAccountNumber(acc: string): string {
  if (!acc || acc.length < 4) return "****";
  return `****${acc.slice(-4)}`;
}

export function maskIFSC(ifsc: string): string {
  if (!ifsc || ifsc.length < 4) return "****";
  return `${ifsc.slice(0, 4)}****`;
}
