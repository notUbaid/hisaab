"use client";

import React, { Component, ErrorInfo, ReactNode } from "react";
import Link from "next/link";
import { AlertOctagon, RefreshCw, Home } from "lucide-react";

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
}

export class ErrorBoundary extends Component<Props, State> {
  public state: State = {
    hasError: false,
    error: null,
    errorInfo: null,
  };

  public static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error, errorInfo: null };
  }

  public componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    this.setState({ error, errorInfo });
    console.error("Uncaught error caught by Hisaab ErrorBoundary:", error, errorInfo);
  }

  private handleReset = () => {
    this.setState({ hasError: false, error: null, errorInfo: null });
  };

  public render() {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback;
      }

      return (
        <div className="min-h-screen w-full flex items-center justify-center p-6 bg-obsidian-900 text-slate-100 font-sans">
          <div className="max-w-xl w-full p-8 rounded-2xl bg-obsidian-800/90 border border-rose-500/30 backdrop-blur-xl shadow-2xl space-y-6">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 rounded-xl bg-rose-500/10 border border-rose-500/30 flex items-center justify-center text-rose-400">
                <AlertOctagon className="w-6 h-6" />
              </div>
              <div>
                <h2 className="text-lg font-semibold text-white tracking-wide">
                  Application Runtime Exception
                </h2>
                <p className="text-xs text-slate-400 font-mono mt-0.5">
                  An unhandled error occurred in the console runtime
                </p>
              </div>
            </div>

            {this.state.error && (
              <div className="p-4 rounded-xl bg-obsidian-950 border border-white/5 space-y-2">
                <div className="text-xs font-mono font-semibold text-rose-400">
                  {this.state.error.name}: {this.state.error.message}
                </div>
                {this.state.error.stack && (
                  <pre className="text-[11px] font-mono text-slate-400 overflow-x-auto max-h-48 whitespace-pre-wrap leading-relaxed select-all">
                    {this.state.error.stack}
                  </pre>
                )}
              </div>
            )}

            <div className="flex items-center gap-3 pt-2">
              <button
                onClick={this.handleReset}
                className="flex items-center gap-2 px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold tracking-wide transition-colors"
              >
                <RefreshCw className="w-3.5 h-3.5" />
                Retry Component Tree
              </button>
              <Link
                href="/"
                className="flex items-center gap-2 px-4 py-2 rounded-lg bg-obsidian-700 hover:bg-obsidian-600 border border-white/10 text-slate-300 hover:text-white text-xs font-medium transition-colors"
              >
                <Home className="w-3.5 h-3.5" />
                Return to Workbench
              </Link>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
