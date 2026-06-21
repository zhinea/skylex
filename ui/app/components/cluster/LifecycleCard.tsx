import type { Cluster } from "~/hooks/useClusters";
import type { Node } from "~/hooks/useNodes";
import type { CommandLog } from "~/hooks/useCommandLogs";
import { Card, CardHeader, CardTitle, CardContent } from "~/components/ui/card";
import { Button } from "~/components/ui/button";
import { Layers } from "lucide-react";
import { FeatureNote } from "./ClusterHelpers";

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
  const busy = pending || logPending || cluster.status === "CLUSTER_STATUS_CREATING" || cluster.status === "CLUSTER_STATUS_DELETING";
  const hasReadyNodes = readyNodes.length > 0;
  const disabledReason = !hasReadyNodes
    ? "Lifecycle controls require at least one connected node with PostgreSQL installed and initialized."
    : busy
      ? "A lifecycle/provisioning command is already pending. Watch command logs for progress."
      : "";

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <Layers className="size-4 text-muted-foreground" />
          Service Lifecycle
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        <FeatureNote title="What this does:">
          Start, pause, or restart PostgreSQL on the ready nodes. Use Pause before maintenance, and Restart after changes that need a full database restart.
        </FeatureNote>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div className="rounded-lg border border-border px-3 py-2 bg-muted/10">
            <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Ready Nodes</div>
            <div className="text-lg font-bold text-foreground mt-0.5">{readyNodes.length}/{nodes.length}</div>
          </div>
          <div className="rounded-lg border border-border px-3 py-2 bg-muted/10">
            <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Running</div>
            <div className="text-lg font-bold text-foreground mt-0.5">{runningNodes.length}</div>
          </div>
          <div className="rounded-lg border border-border px-3 py-2 bg-muted/10">
            <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Stopped</div>
            <div className="text-lg font-bold text-foreground mt-0.5">{stoppedNodes.length}</div>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2 pt-2">
          <Button
            onClick={() => onAction("start")}
            disabled={busy || !hasReadyNodes}
            size="sm"
            className="bg-emerald-600 hover:bg-emerald-700 dark:bg-emerald-500 dark:hover:bg-emerald-600 text-white"
          >
            Start
          </Button>
          <Button
            onClick={() => onAction("pause")}
            disabled={busy || !hasReadyNodes}
            variant="outline"
            size="sm"
          >
            Pause
          </Button>
          <Button
            onClick={() => onAction("restart")}
            disabled={busy || !hasReadyNodes}
            size="sm"
          >
            Restart
          </Button>
          {busy && <span className="text-xs font-medium text-muted-foreground animate-pulse ml-2">Lifecycle command pending...</span>}
        </div>
        {disabledReason && <p className="text-xs text-muted-foreground">{disabledReason}</p>}
        {error && <p className="rounded-lg border border-destructive/20 bg-destructive/10 px-3 py-2 text-xs font-medium text-destructive">{error}</p>}
      </CardContent>
    </Card>
  );
}
