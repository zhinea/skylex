import { useState } from "react";
import { useNavigate } from "react-router";
import { usePauseCluster, useRestartCluster } from "~/hooks/useClusters";
import { useResolveInstallationConflict } from "~/hooks/useNodes";
import { useCommandLogStream } from "~/hooks/useCommandLogStream";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { ClusterStatsGrid } from "~/components/cluster/ClusterStatsGrid";
import { LifecycleCard } from "~/components/cluster/LifecycleCard";
import { InstallationConflictCard } from "~/components/cluster/InstallationConflictCard";
import { InstallationProgressCard } from "~/components/cluster/InstallationProgressCard";
import { useClusterContext } from "./clusters.$id";

export default function ClusterOverviewPage() {
  const { clusterId, cluster, nodes, liveStatus, progressPct } = useClusterContext();
  const navigate = useNavigate();
  // Overview needs logs for the installation-progress and lifecycle cards. The
  // diagnostics page runs its own filtered stream; only one module page is
  // mounted at a time so at most one stream is active.
  const { logs } = useCommandLogStream({ clusterId, filter: { level: "", windowMs: 60 * 60 * 1000, sinceMs: 0, untilMs: 0 } });

  const startCluster = usePauseCluster(); // start maps to the same controls endpoint
  const pauseCluster = usePauseCluster();
  const restartCluster = useRestartCluster();
  const resolveConflict = useResolveInstallationConflict();

  const [clusterAction, setClusterAction] = useState<"start" | "pause" | "restart" | null>(null);
  const [lifecycleError, setLifecycleError] = useState<string | null>(null);
  const [conflictAction, setConflictAction] = useState<{ nodeId: string; action: "PURGE" | "ABORT" } | null>(null);

  const conflictNodes = nodes.filter((n) => n.installationState === "INSTALLATION_STATE_CONFLICT");
  const lifecyclePending = startCluster.isPending || pauseCluster.isPending || restartCluster.isPending;

  function runClusterAction(action: "start" | "pause" | "restart") {
    if (!clusterId) return;
    setLifecycleError(null);
    const options = {
      onSuccess: () => setClusterAction(null),
      onError: (err: unknown) => setLifecycleError(err instanceof Error ? err.message : `Failed to ${action} cluster`),
    };
    if (action === "start") startCluster.mutate(clusterId, options);
    if (action === "pause") pauseCluster.mutate(clusterId, options);
    if (action === "restart") restartCluster.mutate(clusterId, options);
  }

  return (
    <div className="space-y-6">
      {/* Conflict resolution surfaced first — requires user action and is easy to miss at the bottom */}
      {conflictNodes.length > 0 && (
        <InstallationConflictCard
          nodes={conflictNodes}
          pending={resolveConflict.isPending}
          onResolve={(nodeId, action, credentials) => {
            if (action === "ADOPT") {
              resolveConflict.mutate({ nodeId, action, ...credentials });
            } else {
              setConflictAction({ nodeId, action });
            }
          }}
        />
      )}

      <ClusterStatsGrid cluster={cluster} nodes={nodes} />

      {/* Show installation progress while creating or until every node is ready */}
      {(liveStatus === "CREATING" || liveStatus === "CLUSTER_STATUS_CREATING" || progressPct < 100) && (
        <InstallationProgressCard nodes={nodes} logs={logs} />
      )}

      <LifecycleCard
        cluster={cluster}
        nodes={nodes}
        logs={logs}
        pending={lifecyclePending}
        error={lifecycleError}
        onAction={(action) => setClusterAction(action)}
      />

      {/* Nodes Table Section */}
      <section className="v-card rounded-lg overflow-hidden">
        <div className="px-4 py-3 border-b border-border flex items-center justify-between">
          <div className="flex items-center gap-2 text-foreground">
            <span className="material-symbols-outlined text-lg">dns</span>
            <h3 className="text-xs font-semibold">Nodes ({nodes.length})</h3>
          </div>
          <button
            onClick={() => navigate(`/clusters/${clusterId}/diagnostics`)}
            className="text-foreground hover:underline text-[10px] font-bold uppercase tracking-tight cursor-pointer"
          >
            View metrics
          </button>
        </div>
        {nodes.length === 0 ? (
          <p className="text-xs text-muted-foreground py-6 text-center">
            No nodes registered for this cluster.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="bg-neutral-50/30 dark:bg-neutral-900/30 border-b border-border text-muted-foreground font-medium">
                  <th className="px-4 py-2 font-medium uppercase tracking-wider text-[10px]">Hostname</th>
                  <th className="px-4 py-2 font-medium uppercase tracking-wider text-[10px]">Role</th>
                  <th className="px-4 py-2 font-medium uppercase tracking-wider text-[10px]">Port</th>
                  <th className="px-4 py-2 font-medium uppercase tracking-wider text-[10px]">PostgreSQL</th>
                  <th className="px-4 py-2 font-medium uppercase tracking-wider text-[10px]">Agent</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {nodes.map((n) => {
                  const isPrimary = n.role === "NODE_ROLE_PRIMARY";
                  return (
                    <tr key={n.id} className="hover:bg-neutral-50/30 dark:hover:bg-neutral-900/30 transition-colors">
                      <td className="px-4 py-2.5">
                        <div className="flex items-center gap-2">
                          <div className="rounded bg-neutral-100 dark:bg-neutral-800 flex items-center justify-center w-5 h-5 shrink-0 text-foreground">
                            <span className="material-symbols-outlined text-sm">terminal</span>
                          </div>
                          <span className="font-semibold text-foreground text-[11px]">{n.hostname}</span>
                        </div>
                      </td>
                      <td className="px-4 py-2.5">
                        <span className={`text-[9px] font-bold px-1.5 py-0.5 rounded font-mono ${
                          isPrimary
                            ? "bg-neutral-100 dark:bg-neutral-800 text-foreground"
                            : "bg-neutral-50 dark:bg-neutral-900 text-muted-foreground"
                        }`}>
                          {isPrimary ? "PRIMARY" : "REPLICA"}
                        </span>
                      </td>
                      <td className="px-4 py-2.5">
                        <span className="font-mono text-muted-foreground text-[11px]">:{n.port}</span>
                      </td>
                      <td className="px-4 py-2.5">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-foreground text-[11px]">v{n.postgresVersion || cluster.config?.version || "16"}</span>
                          <span className={`px-1 py-0.5 text-[8px] font-bold uppercase rounded border ${
                            n.postgresInstalled && n.postgresDataInitialized
                              ? "bg-emerald-500/10 text-emerald-500 border-emerald-500/20"
                              : "bg-amber-500/10 text-amber-500 border-amber-500/20"
                          }`}>
                            {n.postgresInstalled && n.postgresDataInitialized ? "Ready" : "Uninitialized"}
                          </span>
                        </div>
                      </td>
                      <td className="px-4 py-2.5">
                        <div className="flex items-center gap-1.5">
                          <div className={`w-1.5 h-1.5 rounded-full ${n.agentConnected ? "bg-emerald-500" : "bg-neutral-400"}`}></div>
                          <span className="font-medium text-foreground text-[11px]">
                            {n.agentConnected ? "Connected" : "Disconnected"}
                          </span>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* Lifecycle + conflict confirmation dialogs */}
      <ConfirmDialog
        open={clusterAction === "start"}
        title="Start Cluster"
        message="This will queue PostgreSQL start commands on ready assigned nodes. Already-running instances are treated as success."
        confirmLabel="Start"
        onConfirm={() => runClusterAction("start")}
        onCancel={() => setClusterAction(null)}
      />
      <ConfirmDialog
        open={clusterAction === "pause"}
        title="Pause Cluster"
        message="This will gracefully stop PostgreSQL on ready assigned nodes. Pause the service before deleting a cluster."
        confirmLabel="Pause"
        onConfirm={() => runClusterAction("pause")}
        onCancel={() => setClusterAction(null)}
      />
      <ConfirmDialog
        open={clusterAction === "restart"}
        title="Restart Cluster"
        message="This will queue stop then start commands on ready assigned nodes. PostgreSQL will be temporarily unavailable."
        confirmLabel="Restart"
        onConfirm={() => runClusterAction("restart")}
        onCancel={() => setClusterAction(null)}
      />
      <ConfirmDialog
        open={conflictAction?.action === "PURGE"}
        title="Remove Existing PostgreSQL"
        message="This will stop PostgreSQL, remove native PostgreSQL packages, and delete the configured data directory on this node before reinstalling. This is destructive and cannot be undone."
        confirmLabel="Remove & Reinstall"
        onConfirm={() => {
          if (conflictAction) {
            resolveConflict.mutate({ nodeId: conflictAction.nodeId, action: conflictAction.action });
            setConflictAction(null);
          }
        }}
        onCancel={() => setConflictAction(null)}
      />
      <ConfirmDialog
        open={conflictAction?.action === "ABORT"}
        title="Abort Cluster Creation"
        message="This will mark the cluster creation as failed. Already queued provisioning work will be skipped where possible."
        confirmLabel="Abort Cluster Creation"
        onConfirm={() => {
          if (conflictAction) {
            resolveConflict.mutate({ nodeId: conflictAction.nodeId, action: conflictAction.action });
            setConflictAction(null);
          }
        }}
        onCancel={() => setConflictAction(null)}
      />
    </div>
  );
}
