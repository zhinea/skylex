import React, { createContext, useContext, useState, useCallback } from "react";
import { CheckCircle, AlertCircle, Info, AlertTriangle, X } from "lucide-react";
import { cn } from "~/lib/utils";

export type ToastType = "success" | "error" | "info" | "warning";

export interface Toast {
  id: string;
  title: string;
  description?: string;
  type: ToastType;
  duration?: number;
}

interface ToastContextType {
  toast: (props: Omit<Toast, "id">) => void;
  success: (title: string, description?: string) => void;
  error: (title: string, description?: string) => void;
  info: (title: string, description?: string) => void;
  warning: (title: string, description?: string) => void;
  toasts: Toast[];
  dismiss: (id: string) => void;
}

const ToastContext = createContext<ToastContextType | undefined>(undefined);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const dismiss = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const toast = useCallback(
    ({ title, description, type, duration = 5000 }: Omit<Toast, "id">) => {
      const id = Math.random().toString(36).substring(2, 9);
      setToasts((prev) => [...prev, { id, title, description, type, duration }]);

      if (duration > 0) {
        setTimeout(() => {
          dismiss(id);
        }, duration);
      }
    },
    [dismiss]
  );

  const success = useCallback((title: string, description?: string) => toast({ title, description, type: "success" }), [toast]);
  const error = useCallback((title: string, description?: string) => toast({ title, description, type: "error" }), [toast]);
  const info = useCallback((title: string, description?: string) => toast({ title, description, type: "info" }), [toast]);
  const warning = useCallback((title: string, description?: string) => toast({ title, description, type: "warning" }), [toast]);

  return (
    <ToastContext.Provider value={{ toast, success, error, info, warning, toasts, dismiss }}>
      {children}
      <Toaster />
    </ToastContext.Provider>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error("useToast must be used within a ToastProvider");
  }
  return context;
}

function Toaster() {
  const { toasts, dismiss } = useToast();

  if (toasts.length === 0) return null;

  return (
    <div className="fixed bottom-4 right-4 z-[9999] flex flex-col gap-2.5 w-full max-w-sm pointer-events-none">
      {toasts.map((t) => {
        let Icon = Info;
        let colorClass = "";
        let borderClass = "";
        let progressColor = "";

        switch (t.type) {
          case "success":
            Icon = CheckCircle;
            colorClass = "text-emerald-500 dark:text-emerald-400";
            borderClass = "border-emerald-500/20 dark:border-emerald-500/10";
            progressColor = "bg-emerald-500";
            break;
          case "error":
            Icon = AlertCircle;
            colorClass = "text-destructive dark:text-red-400";
            borderClass = "border-destructive/20 dark:border-destructive/10";
            progressColor = "bg-destructive";
            break;
          case "warning":
            Icon = AlertTriangle;
            colorClass = "text-amber-500 dark:text-amber-400";
            borderClass = "border-amber-500/20 dark:border-amber-500/10";
            progressColor = "bg-amber-500";
            break;
          case "info":
            Icon = Info;
            colorClass = "text-blue-500 dark:text-blue-400";
            borderClass = "border-blue-500/20 dark:border-blue-500/10";
            progressColor = "bg-blue-500";
            break;
        }

        return (
          <div
            key={t.id}
            className={cn(
              "pointer-events-auto relative overflow-hidden rounded-lg border bg-card/90 backdrop-blur-md p-4 shadow-lg flex gap-3 transition-all duration-300 animate-in slide-in-from-bottom-2 fade-in duration-200",
              borderClass
            )}
            role="alert"
          >
            <Icon className={cn("size-5 shrink-0 mt-0.5", colorClass)} />
            <div className="flex-1 space-y-1">
              <h4 className="text-xs font-semibold text-foreground leading-normal">{t.title}</h4>
              {t.description && (
                <p className="text-[11px] text-muted-foreground leading-relaxed">{t.description}</p>
              )}
            </div>
            <button
              onClick={() => dismiss(t.id)}
              className="text-muted-foreground/50 hover:text-foreground shrink-0 size-4 flex items-center justify-center rounded-md hover:bg-muted transition-colors cursor-pointer"
            >
              <X className="size-3" />
            </button>
            {t.duration && t.duration > 0 && (
              <div
                className={cn("absolute bottom-0 left-0 h-0.5 opacity-40", progressColor)}
                style={{
                  width: "100%",
                  animation: `shrink ${t.duration}ms linear forwards`,
                }}
              />
            )}
          </div>
        );
      })}
    </div>
  );
}
