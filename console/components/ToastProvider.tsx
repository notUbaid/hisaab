"use client";

import React, { createContext, useContext, useState, useCallback } from "react";
import { CheckCircle2, AlertTriangle, AlertCircle, Info, X } from "lucide-react";

export type ToastType = "success" | "error" | "warning" | "info";

export interface Toast {
  id: string;
  message: string;
  type: ToastType;
  title?: string;
}

interface ToastContextValue {
  showToast: (message: string, type?: ToastType, title?: string) => void;
  removeToast: (id: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    return {
      showToast: () => {},
      removeToast: () => {},
    };
  }
  return ctx;
}

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const showToast = useCallback((message: string, type: ToastType = "info", title?: string) => {
    const id = `toast-${Date.now()}-${Math.random().toString(36).substring(2, 7)}`;
    setToasts((prev) => [...prev, { id, message, type, title }]);
    setTimeout(() => {
      removeToast(id);
    }, 4500);
  }, [removeToast]);

  return (
    <ToastContext.Provider value={{ showToast, removeToast }}>
      {children}
      {/* Toast Notification Container */}
      <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 max-w-sm w-full pointer-events-none font-sans">
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={`pointer-events-auto p-3.5 rounded-xl border backdrop-blur-md shadow-2xl flex items-start gap-3 animate-in slide-in-from-bottom-3 duration-200 transition-all ${
              toast.type === "success"
                ? "bg-obsidian-900/95 border-emerald-500/40 text-emerald-300"
                : toast.type === "error"
                ? "bg-obsidian-900/95 border-rose-500/40 text-rose-300"
                : toast.type === "warning"
                ? "bg-obsidian-900/95 border-amber-500/40 text-amber-300"
                : "bg-obsidian-900/95 border-cyan-500/40 text-cyan-300"
            }`}
          >
            <div className="shrink-0 mt-0.5">
              {toast.type === "success" && <CheckCircle2 className="w-4 h-4 text-emerald-400" />}
              {toast.type === "error" && <AlertCircle className="w-4 h-4 text-rose-400" />}
              {toast.type === "warning" && <AlertTriangle className="w-4 h-4 text-amber-400" />}
              {toast.type === "info" && <Info className="w-4 h-4 text-cyan-400" />}
            </div>

            <div className="flex-1 space-y-0.5">
              {toast.title && (
                <div className="font-semibold text-xs text-white uppercase tracking-wider">
                  {toast.title}
                </div>
              )}
              <div className="text-xs text-slate-200 font-mono leading-relaxed">{toast.message}</div>
            </div>

            <button
              onClick={() => removeToast(toast.id)}
              className="text-slate-400 hover:text-white p-0.5 rounded transition-colors"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}
