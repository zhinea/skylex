import { Badge as ShadcnBadge } from "~/components/ui/badge";
import { cn } from "~/lib/utils";

const statusMapping: Record<
  string,
  { variant?: "default" | "secondary" | "destructive" | "outline"; className?: string }
> = {
  HEALTHY: { className: "bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:bg-emerald-500/20 dark:text-emerald-400 dark:border-emerald-500/30" },
  COMPLETED: { className: "bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:bg-emerald-500/20 dark:text-emerald-400 dark:border-emerald-500/30" },
  online: { className: "bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:bg-emerald-500/20 dark:text-emerald-400 dark:border-emerald-500/30" },
  Connected: { className: "bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:bg-emerald-500/20 dark:text-emerald-400 dark:border-emerald-500/30" },
  
  FAILED: { variant: "destructive" },
  Disconnected: { variant: "destructive" },
  offline: { variant: "destructive" },
  
  DEGRADED: { className: "bg-amber-500/10 text-amber-600 border-amber-500/20 dark:bg-amber-500/20 dark:text-amber-400 dark:border-amber-500/30" },
  drained: { className: "bg-amber-500/10 text-amber-600 border-amber-500/20 dark:bg-amber-500/20 dark:text-amber-400 dark:border-amber-500/30" },
  
  CREATING: { variant: "secondary" },
  RUNNING: { variant: "secondary" },
  operator: { variant: "secondary" },
  
  PRIMARY: { className: "bg-violet-500/10 text-violet-600 border-violet-500/20 dark:bg-violet-500/20 dark:text-violet-400 dark:border-violet-500/30" },
  admin: { className: "bg-violet-500/10 text-violet-600 border-violet-500/20 dark:bg-violet-500/20 dark:text-violet-400 dark:border-violet-500/30" },
  
  REPLICA: { variant: "outline" },
  DELETING: { variant: "outline" },
  deleting: { variant: "outline" },
  viewer: { variant: "outline" },
};

export function Badge({ label, className = "" }: { label?: string | null; className?: string }) {
  const normalized = label ?? "-";
  const matched = statusMapping[normalized] || statusMapping[normalized.toLowerCase()] || { variant: "outline" };
  
  return (
    <ShadcnBadge
      variant={matched.variant || "default"}
      className={cn("border px-2 py-0.5 rounded-full text-xs font-medium", matched.className, className)}
    >
      {normalized}
    </ShadcnBadge>
  );
}

