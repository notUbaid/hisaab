import "./globals.css";
import React from "react";
import { AppShell } from "@/components/AppShell";

export const metadata = {
  title: "Hisaab — ONDC RSF 2.0 Settlement & Money-Correctness Workbench",
  description:
    "Institutional multi-party reconciliation, double-entry ledger, and statutory tax engine for ONDC RSF 2.0.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className="dark h-full">
      <body className="h-full w-full overflow-hidden bg-obsidian-900 text-slate-100 antialiased font-sans">
        <AppShell>{children}</AppShell>
      </body>
    </html>
  );
}
