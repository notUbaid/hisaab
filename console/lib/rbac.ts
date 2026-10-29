// RBAC Roles for Hisaab Ops Console
// Defined in PRD §8 M6.4 & SECURITY.md

export type Role = "viewer" | "operator" | "approver";

export interface UserSession {
  userId: string;
  email: string;
  role: Role;
  expiresAt: number;
}

export const ROLE_HIERARCHY: Record<Role, number> = {
  viewer: 1,
  operator: 2,
  approver: 3,
};

export function hasRole(session: UserSession | null, requiredRole: Role): boolean {
  if (!session) return false;
  return ROLE_HIERARCHY[session.role] >= ROLE_HIERARCHY[requiredRole];
}

export function maskAccountNumber(acc: string): string {
  if (!acc || acc.length < 4) return "****";
  return `****${acc.slice(-4)}`;
}

export function maskIFSC(ifsc: string): string {
  if (!ifsc || ifsc.length < 4) return "****";
  return `${ifsc.slice(0, 4)}****`;
}

export function formatPaiseToINR(paise: number | bigint): string {
  const num = typeof paise === "bigint" ? Number(paise) : paise;
  const inr = (num / 100).toLocaleString("en-IN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return `₹${inr}`;
}
