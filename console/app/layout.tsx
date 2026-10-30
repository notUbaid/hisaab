import "./globals.css";
import React from "react";

export const metadata = {
  title: "Hisaab — ONDC RSF 2.0 Settlement & Correctness Workbench",
  description: "High-density money-correctness and multi-party settlement simulator for ONDC RSF 2.0",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className="dark">
      <body className="h-screen w-screen overflow-hidden bg-zinc-950 text-zinc-100 flex flex-col antialiased">
        <main className="flex-1 min-h-0 w-full overflow-hidden flex flex-col">{children}</main>
      </body>
    </html>
  );
}
