import "./globals.css";
import React from "react";
import Link from "next/link";

export const metadata = {
  title: "Hisaab — ONDC RSF 2.0 Settlement & Correctness Lab",
  description: "Production-grade money-correctness engine and settlement simulator for ONDC RSF 2.0",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className="dark">
      <body className="min-h-screen bg-zinc-950 text-zinc-100 flex flex-col">
        {/* Top Header */}
        <header className="border-b border-zinc-800 bg-zinc-900/50 backdrop-blur px-6 py-3 flex items-center justify-between sticky top-0 z-50">
          <div className="flex items-center gap-6">
            <Link href="/" className="flex items-center gap-2">
              <span className="font-bold text-xl tracking-tight text-white flex items-center gap-1.5">
                <span className="h-3 w-3 rounded-full bg-emerald-500 inline-block animate-pulse"></span>
                Hisaab <span className="text-xs px-2 py-0.5 rounded bg-zinc-800 text-zinc-400 font-mono">RSF 2.0</span>
              </span>
            </Link>

            <nav className="flex items-center gap-1 text-sm font-medium">
              <Link
                href="/"
                className="px-3 py-1.5 rounded-md hover:bg-zinc-800 text-zinc-300 hover:text-white transition"
              >
                Dashboard
              </Link>
              <Link
                href="/timeline"
                className="px-3 py-1.5 rounded-md hover:bg-zinc-800 text-zinc-300 hover:text-white transition"
              >
                Settlement Timeline
              </Link>
              <Link
                href="/exceptions"
                className="px-3 py-1.5 rounded-md hover:bg-zinc-800 text-zinc-300 hover:text-white transition"
              >
                Exception Queue
              </Link>
              <Link
                href="/playground"
                className="px-3 py-1.5 rounded-md hover:bg-zinc-800 text-zinc-300 hover:text-white transition"
              >
                Scenario Playground
              </Link>
              <Link
                href="/scenarios"
                className="px-3 py-1.5 rounded-md hover:bg-zinc-800 text-zinc-300 hover:text-white transition flex items-center gap-1.5"
              >
                Scenario Studio
                <span className="text-[10px] bg-emerald-950 border border-emerald-800 text-emerald-400 px-1.5 py-0.2 rounded font-mono">DSL</span>
              </Link>
            </nav>
          </div>

          <div className="flex items-center gap-3 text-xs">
            <span className="text-zinc-500 font-mono">API: Asia/Kolkata (IST)</span>
            <span className="px-2.5 py-1 rounded-full bg-emerald-950/80 border border-emerald-800 text-emerald-400 font-medium">
              Simulator Online
            </span>
          </div>
        </header>

        {/* Main Content */}
        <main className="flex-1 max-w-7xl w-full mx-auto p-6">{children}</main>

        {/* Footer Disclaimer per PRD §4 */}
        <footer className="border-t border-zinc-900 py-4 px-6 text-center text-xs text-zinc-500">
          <p>
            Hisaab is an independent, open-source money-correctness simulator. Label:{" "}
            <strong className="text-zinc-400">BRD-faithful, not production-verified</strong>. Synthetic data only. Does
            not touch real money.
          </p>
        </footer>
      </body>
    </html>
  );
}
