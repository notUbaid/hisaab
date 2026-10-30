import "./globals.css";
import React from "react";

export const metadata = {
  title: "Hisaab — ONDC RSF 2.0 Settlement Workbench",
  description:
    "High-density money-correctness and multi-party settlement simulator for ONDC RSF 2.0",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className="dark" style={{ height: "100%" }}>
      <body
        className="bg-zinc-950 text-zinc-100 antialiased"
        style={{
          height: "100vh",
          overflow: "hidden",
          display: "flex",
          flexDirection: "column",
          fontFamily: "var(--font-sans)",
        }}
      >
        {/* ── Global Nav Bar ── */}
        <header
          className="shrink-0 flex items-center justify-between px-4 border-b"
          style={{
            height: "var(--topbar-h)",
            background: "var(--color-surface)",
            borderColor: "var(--color-border)",
          }}
        >
          {/* Brand */}
          <div className="flex items-center gap-3">
            {/* Logotype mark */}
            <div className="flex items-center gap-2">
              <div
                className="flex items-center justify-center rounded"
                style={{
                  width: 22,
                  height: 22,
                  background: "rgba(34,197,94,0.15)",
                  border: "1px solid rgba(34,197,94,0.3)",
                }}
              >
                {/* Ledger balance icon — two horizontal bars */}
                <svg width="12" height="10" viewBox="0 0 12 10" fill="none" aria-hidden="true">
                  <rect x="0" y="1" width="5" height="1.5" rx="0.75" fill="#22c55e" />
                  <rect x="7" y="1" width="5" height="1.5" rx="0.75" fill="#22c55e" opacity="0.5" />
                  <rect x="0" y="4.25" width="12" height="1.5" rx="0.75" fill="#22c55e" opacity="0.3" />
                  <rect x="0" y="7.5" width="7" height="1.5" rx="0.75" fill="#22c55e" />
                  <rect x="9" y="7.5" width="3" height="1.5" rx="0.75" fill="#22c55e" opacity="0.5" />
                </svg>
              </div>
              <span
                className="font-semibold tracking-tight"
                style={{ fontSize: 14, color: "#f1f5f9" }}
              >
                Hisaab
              </span>
            </div>

            <div
              style={{ width: 1, height: 16, background: "var(--color-border)" }}
            />

            <span
              className="font-mono"
              style={{ fontSize: 10, color: "#475569", letterSpacing: "0.06em" }}
            >
              ONDC RSF 2.0 · SETTLEMENT WORKBENCH
            </span>
          </div>

          {/* Right — build / version badge */}
          <div className="flex items-center gap-2">
            <span
              className="badge badge-positive"
              style={{ fontSize: 9 }}
            >
              LIVE ENGINE
            </span>
            <span
              className="font-mono"
              style={{ fontSize: 10, color: "#334155" }}
            >
              v0.1.0
            </span>
          </div>
        </header>

        {/* ── Page Content ── */}
        <main className="flex-1 min-h-0 w-full overflow-hidden flex flex-col">
          {children}
        </main>
      </body>
    </html>
  );
}
