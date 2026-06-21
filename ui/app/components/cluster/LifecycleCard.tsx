import type { Cluster } from "~/hooks/useClusters";
import type { Node } from "~/hooks/useNodes";
import type { CommandLog } from "~/hooks/useCommandLogs";

function hasPendingLifecycleCommand(logs: CommandLog[]) {
  let pending = false;
  for (const log of logs) {
    const message = log.message.toLowerCase();
    if (message.includes("executing command: pg_start") || message.includes("executing command: pg_stop")) {
      pending = true;
    }
    if (message.includes("command finished") && pending) {
      pending = false;
    }
  }
  return pending;
}

interface LifecycleCardProps {
  cluster: Cluster;
  nodes: Node[];
  logs: CommandLog[];
  pending: boolean;
  error: string | null;
  onAction: (action: "start" | "pause" | "restart") => void;
}

export function LifecycleCard({
  cluster,
  nodes,
  logs,
  pending,
  error,
  onAction,
}: LifecycleCardProps) {
  const readyNodes = nodes.filter((node) => node.postgresInstalled && node.postgresDataInitialized);
  const runningNodes = nodes.filter((node) => node.statusDetail === "healthy" || node.statusDetail === "running" || node.statusDetail === "syncing_replica" || node.status === "online");
  const stoppedNodes = nodes.filter((node) => node.statusDetail === "stopped" || node.status === "offline");
  const logPending = hasPendingLifecycleCommand(logs);
  const busy = pending || logPending || cluster.status === "CLUSTER_STATUS_CREATING" || cluster.status === "CREATING" || cluster.status === "CLUSTER_STATUS_DELETING" || cluster.status === "DELETING";
  const hasReadyNodes = readyNodes.length > 0;
  const disabledReason = !hasReadyNodes
    ? "Lifecycle controls require at least one connected node with PostgreSQL installed and initialized."
    : busy
      ? "A lifecycle/provisioning command is already pending. Watch command logs for progress."
      : "";

  return (
    <section className="v-card rounded-lg overflow-hidden">
      <div className="px-4 py-3 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div className="flex flex-wrap items-center gap-4">
          <div className="flex items-center gap-1.5">
            <span className="material-symbols-outlined text-lg text-foreground">layers</span>
            <h3 className="text-xs font-semibold text-foreground">Service Lifecycle</h3>
            <span className="material-symbols-outlined text-xs text-neutral-400 cursor-help"
                title="Lifecycle operations allow you to start, pause, or restart PostgreSQL.">info</span>
          </div>
          <div className="flex items-center gap-3 border-l border-border pl-4">
            <div className="flex items-center gap-1">
              <span className="text-[10px] text-muted-foreground uppercase">Ready:</span>
              <span className="text-[10px] font-bold text-foreground">{readyNodes.length}</span>
            </div>
            <div className="flex items-center gap-1">
              <span className="text-[10px] text-muted-foreground uppercase">Running:</span>
              <span className="text-[10px] font-bold text-foreground">{runningNodes.length}</span>
            </div>
            <div className="flex items-center gap-1">
              <span className="text-[10px] text-muted-foreground uppercase">Stopped:</span>
              <span className="text-[10px] font-bold text-muted-foreground">{stoppedNodes.length}</span>
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {busy && <span className="text-[10px] font-medium text-muted-foreground animate-pulse mr-2">Lifecycle pending...</span>}
          <button
              onClick={() => onAction("start")}
              disabled={busy || !hasReadyNodes}
              className="bg-foreground text-background text-[11px] font-medium px-3 py-1 rounded hover:opacity-85 disabled:opacity-40 transition-all flex items-center gap-1 cursor-pointer">
              <span className="material-symbols-outlined text-sm">play_arrow</span>
              Start
          </button>
          <button
              onClick={() => onAction("pause")}
              disabled={busy || !hasReadyNodes}
              className="bg-transparent border border-border text-foreground text-[11px] font-medium px-3 py-1 rounded hover:bg-neutral-50 dark:hover:bg-neutral-900 disabled:opacity-40 transition-all flex items-center gap-1 cursor-pointer">
              <span className="material-symbols-outlined text-sm">pause</span>
              Pause
          </button>
          <button
              onClick={() => onAction("restart")}
              disabled={busy || !hasReadyNodes}
              className="bg-transparent border border-border text-foreground text-[11px] font-medium px-3 py-1 rounded hover:bg-neutral-50 dark:hover:bg-neutral-900 disabled:opacity-40 transition-all flex items-center gap-1 cursor-pointer">
              <span className="material-symbols-outlined text-sm">replay</span>
              Restart
          </button>
        </div>
      </div>
      {(disabledReason || error) && (
        <div className="px-4 pb-3 pt-1.5 border-t border-border bg-muted/10 space-y-1">
          {disabledReason && <p className="text-xs text-muted-foreground">{disabledReason}</p>}
          {error && <p className="text-xs font-semibold text-destructive">{error}</p>}
        </div>
      )}
    </section>
  );
}
