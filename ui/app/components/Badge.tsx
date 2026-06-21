import { Badge as ShadcnBadge } from "~/components/ui/badge";
import { cn } from "~/lib/utils";

interface StatusBadgeStyle {
  variant?: "default" | "secondary" | "destructive" | "outline";
  className?: string;
  dotColor?: string;
}

const statusMapping: Record<string, StatusBadgeStyle> = {
  HEALTHY: { dotColor: "bg-emerald-500", className: "bg-emerald-50/60 text-emerald-700 border-emerald-200/50 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50" },
  COMPLETED: { dotColor: "bg-emerald-500", className: "bg-emerald-50/60 text-emerald-700 border-emerald-200/50 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50" },
  online: { dotColor: "bg-emerald-500", className: "bg-emerald-50/60 text-emerald-700 border-emerald-200/50 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50" },
  Connected: { dotColor: "bg-emerald-500", className: "bg-emerald-50/60 text-emerald-700 border-emerald-200/50 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50" },

  FAILED: { dotColor: "bg-rose-500", className: "bg-rose-50/60 text-rose-700 border-rose-200/50 dark:bg-rose-950/20 dark:text-rose-400 dark:border-rose-800/50" },
  Disconnected: { dotColor: "bg-rose-500", className: "bg-rose-50/60 text-rose-700 border-rose-200/50 dark:bg-rose-950/20 dark:text-rose-400 dark:border-rose-800/50" },
  offline: { dotColor: "bg-rose-500", className: "bg-rose-50/60 text-rose-700 border-rose-200/50 dark:bg-rose-950/20 dark:text-rose-400 dark:border-rose-800/50" },

  DEGRADED: { dotColor: "bg-orange-500", className: "bg-orange-50/60 text-orange-700 border-orange-200/50 dark:bg-orange-950/20 dark:text-orange-400 dark:border-orange-800/50" },
  drained: { dotColor: "bg-amber-500", className: "bg-amber-50/60 text-amber-700 border-amber-200/50 dark:bg-amber-950/20 dark:text-amber-400 dark:border-amber-800/50" },

  CREATING: { dotColor: "bg-blue-500 animate-pulse", className: "bg-blue-50/60 text-blue-700 border-blue-200/50 dark:bg-blue-900/20 dark:text-blue-400 dark:border-blue-800/50" },
  RUNNING: { dotColor: "bg-blue-500", className: "bg-blue-50/60 text-blue-700 border-blue-200/50 dark:bg-blue-900/20 dark:text-blue-400 dark:border-blue-800/50" },
  operator: { dotColor: "bg-neutral-500", className: "bg-neutral-50/60 text-neutral-700 border-neutral-200/50 dark:bg-neutral-900/20 dark:text-neutral-300 dark:border-neutral-800/50" },

  PRIMARY: { dotColor: "bg-violet-500", className: "bg-violet-50/60 text-violet-700 border-violet-200/50 dark:bg-violet-950/20 dark:text-violet-400 dark:border-violet-800/50" },
  admin: { dotColor: "bg-violet-500", className: "bg-violet-50/60 text-violet-700 border-violet-200/50 dark:bg-violet-950/20 dark:text-violet-400 dark:border-violet-800/50" },

  REPLICA: { className: "bg-neutral-50 text-neutral-600 border-neutral-200 dark:bg-neutral-900/20 dark:text-neutral-400 dark:border-neutral-800" },
  DELETING: { dotColor: "bg-rose-400 animate-pulse", className: "bg-rose-50/60 text-rose-600 border-rose-200/50 dark:bg-rose-950/20 dark:text-rose-400 dark:border-rose-800/50" },
  deleting: { dotColor: "bg-rose-400 animate-pulse", className: "bg-rose-50/60 text-rose-600 border-rose-200/50 dark:bg-rose-950/20 dark:text-rose-400 dark:border-rose-800/50" },
  viewer: { className: "bg-neutral-50 text-neutral-600 border-neutral-200 dark:bg-neutral-900/20 dark:text-neutral-400 dark:border-neutral-800" },
};

export function Badge({ label, className = "" }: { label?: string | null; className?: string }) {
  const rawLabel = label ?? "-";
  const normalized = rawLabel.replace(/^CLUSTER_STATUS_/, "");
  const matched = statusMapping[normalized] || statusMapping[normalized.toLowerCase()] || { className: "bg-neutral-50 text-neutral-600 border-neutral-200 dark:bg-neutral-900/20 dark:text-neutral-400 dark:border-neutral-800" };

  return (
    <ShadcnBadge
      variant="outline"
      className={cn(
        "gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium tracking-wide border transition-all duration-200",
        matched.className,
        className
      )}
    >
      {matched.dotColor && (
        <span className={cn("size-1.5 rounded-full shrink-0", matched.dotColor)} />
      )}
      {normalized}
    </ShadcnBadge>
  );
}
