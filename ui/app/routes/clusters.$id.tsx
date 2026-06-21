import { useEffect, useRef, useState } from "react";
import { useParams, Link } from "react-router";
import { useCluster, useDeleteCluster, usePauseCluster, useRestartCluster, useRestartNode } from "~/hooks/useClusters";
import { useNodes, useRejoinNode, useResolveInstallationConflict } from "~/hooks/useNodes";
import { useCommandLogs, type CommandLog } from "~/hooks/useCommandLogs";
import { Badge } from "~/components/Badge";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { PageSpinner } from "~/components/Spinner";
import { AgentStatus } from "~/components/AgentStatus";
import { LayoutDashboard, Link2, Settings as SettingsIcon, ShieldAlert, Layers, ArrowLeft } from "lucide-react";
import { Button } from "~/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "~/components/ui/table";

// Helper components imported from the subcomponents folder
import { PgStatusBadges, levelColor, statusDetailColor } from "~/components/cluster/ClusterHelpers";
import { ClusterStatsGrid } from "~/components/cluster/ClusterStatsGrid";
import { LifecycleCard } from "~/components/cluster/LifecycleCard";
import { PostgreSQLConnectionCard } from "~/components/cluster/PostgreSQLConnectionCard";
import { SettingsCard } from "~/components/cluster/SettingsCard";
import { InstallationConflictCard } from "~/components/cluster/InstallationConflictCard";
import { InstallationProgressCard } from "~/components/cluster/InstallationProgressCard";

export default function ClusterDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: clusterData, isLoading: clusterLoading } = useCluster(id || "");
  const { data: nodesData } = useNodes(id || "");
  const { data: logsData } = useCommandLogs(id || "");
  const startCluster = usePauseCluster(); // Note: useStartCluster mapping
  const pauseCluster = usePauseCluster();
  const restartCluster = useRestartCluster();
  const deleteCluster = useDeleteCluster();
  const restartNode = useRestartNode();
  const rejoinNode = useRejoinNode();
  const resolveConflict = useResolveInstallationConflict();
  const logsEndRef = useRef<HTMLDivElement>(null);

  const [actionNodeId, setActionNodeId] = useState<string | null>(null);
  const [actionType, setActionType] = useState<"restart" | "rejoin" | null>(null);
  const [clusterAction, setClusterAction] = useState<"start" | "pause" | "restart" | null>(null);
  const [deleteClusterOpen, setDeleteClusterOpen] = useState(false);
  const [lifecycleError, setLifecycleError] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [conflictAction, setConflictAction] = useState<{ nodeId: string; action: "PURGE" | "ABORT" } | null>(null);
  const [activeTab, setActiveTab] = useState<"overview" | "connect" | "settings" | "diagnostics">("overview");

  const logs: CommandLog[] = logsData?.logs ?? [];

  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  if (clusterLoading) return <PageSpinner />;

  const cluster = clusterData?.cluster;
  if (!cluster) {
    return (
      <div className="text-center py-12">
        <h3 className="text-lg font-medium text-foreground">Cluster not found</h3>
        <Link to="/clusters" className="mt-4 text-primary hover:underline text-sm">Back to Clusters</Link>
      </div>
    );
  }

  const nodes = nodesData?.nodes || [];
  const conflictNodes = nodes.filter((n) => n.installationState === "INSTALLATION_STATE_CONFLICT");
  const onlineNodes = nodes.filter((n) => n.postgresInstalled && n.postgresDataInitialized);
  const totalNodes = nodes.length;

  // Compute overall progress
  const progressPct = totalNodes > 0 ? Math.round((onlineNodes.length / totalNodes) * 100) : 0;

  // Find latest error log
  const lastErrorLog = logs.filter((l) => l.level === "error").slice(-1)[0] ?? null;

  // Suggest fix from error message
  const suggestedFix = (() => {
    if (!lastErrorLog) return null;
    const msg = lastErrorLog.message.toLowerCase();
    if (msg.includes("permission denied")) return "Check data directory ownership on the node.";
    if (msg.includes("port") && (msg.includes("in use") || msg.includes("already")))
      return "Stop the existing PostgreSQL process on the node or change the port.";
    if (msg.includes("connection refused"))
      return "Ensure the primary node is running and reachable from the replica.";
    if (msg.includes("not installed") || msg.includes("not found"))
      return "Install PostgreSQL on the node and re-register the agent.";
    return null;
  })();

  const lifecyclePending = startCluster.isPending || pauseCluster.isPending || restartCluster.isPending;

  function runClusterAction(action: "start" | "pause" | "restart") {
    if (!id) return;
    setLifecycleError(null);
    const options = {
      onSuccess: () => setClusterAction(null),
      onError: (err: unknown) => setLifecycleError(err instanceof Error ? err.message : `Failed to ${action} cluster`),
    };
    if (action === "start") startCluster.mutate(id, options); // start matches controls mappings
    if (action === "pause") pauseCluster.mutate(id, options);
    if (action === "restart") restartCluster.mutate(id, options);
  }

  function deleteClusterAfterStop() {
    if (!id) return;
    setDeleteError(null);
    deleteCluster.mutate(id, {
      onSuccess: () => setDeleteClusterOpen(false),
      onError: (err) => {
        const message = err instanceof Error ? err.message : "Failed to delete cluster";
        setDeleteError(message.includes("running") || message.includes("pause") || message.includes("stop")
          ? `${message} Pause/stop the service first, wait for nodes to show stopped, then delete again.`
          : message);
      },
    });
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-2">
        <Link to="/clusters" className="inline-flex items-center gap-1.5 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors w-fit">
          <ArrowLeft className="size-3.5" /> Back to Clusters
        </Link>
        <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4 border-b border-border/60 pb-5">
          <div className="space-y-1.5">
            <div className="flex items-center gap-3">
              <h2 className="text-2xl font-bold tracking-tight text-foreground">{cluster.name}</h2>
              <Badge label={cluster.status} className="bg-primary/10 text-primary border-primary/20" />
            </div>
            <p className="text-xs text-muted-foreground">
              Managed PostgreSQL cluster with {nodes.length} node{nodes.length === 1 ? "" : "s"} • service location is <span className="font-semibold text-foreground">{cluster.serviceLocation === "SERVICE_LOCATION_DOCKER" || cluster.config?.serviceLocation === "SERVICE_LOCATION_DOCKER" ? "Dockerized" : "Native"}</span>
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button
              onClick={() => setDeleteClusterOpen(true)}
              variant="destructive"
              size="sm"
            >
              Delete Cluster
            </Button>
          </div>
        </div>
      </div>

      {/* Sub-menu Tabs */}
      <div className="flex gap-6 border-b border-border/60">
        {(["overview", "connect", "settings", "diagnostics"] as const).map((tab) => {
          const isActive = activeTab === tab;
          const label = tab.charAt(0).toUpperCase() + tab.slice(1);
          let Icon = LayoutDashboard;
          if (tab === "connect") Icon = Link2;
          if (tab === "settings") Icon = SettingsIcon;
          if (tab === "diagnostics") Icon = ShieldAlert;
          
          return (
            <button
              key={tab}
              onClick={() => setActiveTab(tab)}
              className={`flex items-center gap-2 pb-3 text-sm font-medium border-b-2 -mb-[2px] transition-all cursor-pointer focus:outline-none ${
                isActive
                  ? "border-primary text-foreground font-semibold"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
            >
              <Icon className="size-4" />
              {tab === "diagnostics" ? "Diagnostics & Logs" : label}
            </button>
          );
        })}
      </div>

      {/* Tab Contents */}
      {activeTab === "overview" && (
        <div className="space-y-6">
          {/* Neon-Style Stats Grid */}
          <ClusterStatsGrid cluster={cluster} nodes={nodes} />

          {/* Show installation progress if not fully healthy or in CREATING status */}
          {(cluster.status === "CREATING" || progressPct < 100) && (
            <InstallationProgressCard nodes={nodes} logs={logs} />
          )}

          {conflictNodes.length > 0 && (
            <InstallationConflictCard
              nodes={conflictNodes}
              pending={resolveConflict.isPending}
              onResolve={(nodeId, action) => {
                if (action === "ADOPT") {
                  resolveConflict.mutate({ nodeId, action });
                } else {
                  setConflictAction({ nodeId, action });
                }
              }}
            />
          )}

          <LifecycleCard
            cluster={cluster}
            nodes={nodes}
            logs={logs}
            pending={lifecyclePending}
            error={lifecycleError}
            onAction={(action) => setClusterAction(action)}
          />

          <Card className="shadow-xs">
            <CardHeader className="border-b border-border/60 pb-4">
              <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
                <Layers className="size-4 text-muted-foreground" />
                Nodes ({nodes.length})
              </CardTitle>
            </CardHeader>
            <CardContent className="pt-6">
              {nodes.length === 0 ? (
                <p className="text-xs text-muted-foreground py-4 text-center">
                  No nodes registered for this cluster.
                </p>
              ) : (
                <div className="overflow-x-auto rounded-lg border border-border">
                  <Table>
                    <TableHeader>
                      <TableRow className="bg-muted/30">
                        <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Hostname</TableHead>
                        <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Role</TableHead>
                        <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Address</TableHead>
                        <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">PostgreSQL</TableHead>
                        <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Agent</TableHead>
                        <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Version</TableHead>
                        <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4 text-right">Last Seen</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {nodes.map((n) => (
                        <TableRow key={n.id} className="hover:bg-muted/10 border-b border-border/40 last:border-none">
                          <TableCell className="px-4 py-2.5 font-semibold text-foreground">{n.hostname}</TableCell>
                          <TableCell className="px-4 py-2.5"><Badge label={n.role} /></TableCell>
                          <TableCell className="px-4 py-2.5 font-mono text-xs text-foreground">{n.address}:{n.port}</TableCell>
                          <TableCell className="px-4 py-2.5">
                            <PgStatusBadges
                              installed={n.postgresInstalled}
                              version={n.postgresVersion}
                              dataInitialized={n.postgresDataInitialized}
                            />
                          </TableCell>
                          <TableCell className="px-4 py-2.5">
                            <AgentStatus connected={n.agentConnected} latencyMs={n.agentLatencyMs} />
                          </TableCell>
                          <TableCell className="px-4 py-2.5 text-foreground/80 text-xs">{n.agentVersion || "-"}</TableCell>
                          <TableCell className="px-4 py-2.5 text-muted-foreground text-xs text-right">
                            {n.lastSeen ? new Date(n.lastSeen).toLocaleString() : "-"}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </CardContent>
          </Card>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <Card className="shadow-xs">
              <CardHeader className="border-b border-border/60 pb-4">
                <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
                  <SettingsIcon className="size-4 text-muted-foreground" />
                  Configuration
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-6">
                <dl className="space-y-2 text-xs">
                  <div className="flex justify-between py-1.5 border-b border-border/40">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">Engine</dt>
                    <dd className="text-foreground font-semibold">{cluster.config?.engine || "POSTGRESQL"}</dd>
                  </div>
                  <div className="flex justify-between py-1.5 border-b border-border/40">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">Version</dt>
                    <dd className="text-foreground font-semibold">{cluster.config?.version || "16"}</dd>
                  </div>
                  <div className="flex justify-between py-1.5 border-b border-border/40">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">Service Location</dt>
                    <dd className="text-foreground font-semibold">
                      {cluster.serviceLocation === "SERVICE_LOCATION_DOCKER" || cluster.config?.serviceLocation === "SERVICE_LOCATION_DOCKER"
                        ? "Dockerized"
                        : "Native"}
                    </dd>
                  </div>
                  <div className="flex justify-between py-1.5 border-b border-border/40">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">Replication</dt>
                    <dd className="text-foreground font-semibold">{cluster.config?.replicationMode || "ASYNC"}</dd>
                  </div>
                  <div className="flex justify-between py-1.5 border-b border-border/40">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">Replicas</dt>
                    <dd className="text-foreground font-semibold">{cluster.config?.replicaCount || 0}</dd>
                  </div>
                  <div className="flex justify-between py-1.5 border-b border-border/40">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">PITR</dt>
                    <dd className="text-foreground font-semibold">{cluster.config?.pitrEnabled ? "Enabled" : "Disabled"}</dd>
                  </div>
                  <div className="flex justify-between py-1.5">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">Created</dt>
                    <dd className="text-foreground font-semibold">{new Date(cluster.createdAt).toLocaleString()}</dd>
                  </div>
                </dl>
              </CardContent>
            </Card>

            <Card className="shadow-xs">
              <CardHeader className="border-b border-border/60 pb-4">
                <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
                  <Layers className="size-4 text-muted-foreground" />
                  Labels
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-6">
                {cluster.config?.labels && Object.keys(cluster.config.labels).length > 0 ? (
                  <div className="flex flex-wrap gap-2">
                    {Object.entries(cluster.config.labels).map(([k, v]) => (
                      <span key={k} className="px-2.5 py-1 bg-muted/40 text-foreground border border-border rounded-md text-xs font-mono">
                        {k}: {v}
                      </span>
                    ))}
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">No labels configured</p>
                )}
              </CardContent>
            </Card>
          </div>
        </div>
      )}

      {activeTab === "connect" && (
        <div className="space-y-6">
          {nodes.length > 0 ? (
            <PostgreSQLConnectionCard clusterId={id || ""} nodes={nodes} cluster={cluster} />
          ) : (
            <p className="text-xs text-muted-foreground py-8 text-center bg-card border rounded-xl shadow-xs">
              No nodes configured. Add nodes to view connection details.
            </p>
          )}
        </div>
      )}

      {activeTab === "settings" && (
        <div className="space-y-6">
          <SettingsCard clusterId={id || ""} />
        </div>
      )}

      {activeTab === "diagnostics" && (
        <div className="space-y-6">
          <Card className="shadow-xs">
            <CardHeader className="border-b border-border/60 pb-4">
              <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
                <ShieldAlert className="size-4 text-muted-foreground" />
                Diagnostics
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-5 pt-6">
              {/* Overall progress bar */}
              <div>
                <div className="flex justify-between text-xs mb-2">
                  <span className="font-semibold text-muted-foreground uppercase tracking-wider">Cluster Progress</span>
                  <span className="text-foreground font-semibold">
                    {onlineNodes.length}/{totalNodes} nodes healthy
                  </span>
                </div>
                <div className="w-full bg-muted rounded-full h-2 overflow-hidden border border-border/50">
                  <div
                    className={`h-2 rounded-full transition-all duration-500 ${
                      progressPct === 100
                        ? "bg-emerald-500"
                        : progressPct > 50
                          ? "bg-primary"
                          : "bg-amber-500"
                    }`}
                    style={{ width: `${progressPct}%` }}
                  />
                </div>
              </div>

              {/* Last error + suggested fix */}
              {lastErrorLog && (
                <div className="flex items-start gap-3 px-4 py-3 rounded-lg bg-destructive/10 border border-destructive/20 text-xs text-destructive/90 leading-relaxed">
                  <span className="text-destructive font-bold mt-0.5">✗</span>
                  <div>
                    <p className="font-semibold text-destructive mb-1">Last Error</p>
                    <p className="font-mono bg-black/5 dark:bg-black/20 p-2 rounded border border-destructive/15 break-all">
                      [{lastErrorLog.hostname || lastErrorLog.nodeId?.slice(0, 8)}] {lastErrorLog.message}
                    </p>
                    {suggestedFix && (
                      <p className="mt-2 text-muted-foreground leading-normal">
                        <strong className="text-foreground">Suggested fix:</strong> {suggestedFix}
                      </p>
                    )}
                  </div>
                </div>
              )}

              {/* Per-node status with actions */}
              {nodes.length > 0 && (
                <div className="overflow-x-auto rounded-lg border border-border">
                  <Table>
                    <TableHeader>
                      <TableRow className="bg-muted/30">
                        <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Node</TableHead>
                        <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Status</TableHead>
                        <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4 text-right">Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {nodes.map((n) => (
                        <TableRow key={n.id} className="hover:bg-muted/10 border-b border-border/40 last:border-none">
                          <TableCell className="px-4 py-2.5">
                            <div className="text-foreground font-semibold">{n.hostname}</div>
                            <div className="text-[10px] text-muted-foreground uppercase tracking-wider font-mono">{n.role}</div>
                          </TableCell>
                          <TableCell className="px-4 py-2.5">
                            {n.statusDetail ? (
                              <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${statusDetailColor(n.statusDetail)}`}>
                                {n.statusDetail.replace(/_/g, " ")}
                              </span>
                            ) : n.installationState === "INSTALLATION_STATE_CONFLICT" ? (
                              <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${statusDetailColor("installation_conflict")}`}>
                                installation conflict
                              </span>
                            ) : (
                              <Badge label={n.role} />
                            )}
                          </TableCell>
                          <TableCell className="px-4 py-2.5 text-right">
                            <div className="flex justify-end gap-3">
                              <Button
                                variant="ghost"
                                size="xs"
                                onClick={() => { setActionNodeId(n.id); setActionType("restart"); }}
                                className="text-xs text-primary hover:underline font-semibold"
                              >
                                Restart
                              </Button>
                              {n.role === "NODE_ROLE_REPLICA" && (
                                <Button
                                  variant="ghost"
                                  size="xs"
                                  onClick={() => { setActionNodeId(n.id); setActionType("rejoin"); }}
                                  className="text-xs text-muted-foreground hover:text-foreground hover:underline font-semibold"
                                >
                                  Re-sync
                                </Button>
                              )}
                            </div>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </CardContent>
          </Card>

          <Card className="shadow-xs">
            <CardHeader className="border-b border-border/60 pb-4">
              <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
                <ShieldAlert className="size-4 text-muted-foreground" />
                Command Logs
              </CardTitle>
            </CardHeader>
            <CardContent className="pt-6">
              {logs.length === 0 ? (
                <p className="text-xs text-muted-foreground py-4 text-center">
                  No command logs yet. Logs appear while the agent executes commands.
                </p>
              ) : (
                <div className="overflow-x-auto max-h-[500px] overflow-y-auto font-mono text-[11px] rounded-lg bg-zinc-950 text-zinc-200 border border-zinc-800 p-3 space-y-1">
                  <table className="w-full">
                    <tbody>
                      {logs.map((log) => (
                        <tr key={log.id} className="border-b border-zinc-900/50 last:border-0 hover:bg-zinc-900/25">
                          <td className="py-1 pr-3 whitespace-nowrap text-zinc-500">
                            {new Date(Number(log.timestampMs)).toLocaleTimeString()}
                          </td>
                          <td className="py-1 pr-3 whitespace-nowrap text-zinc-400">
                            {log.hostname || log.nodeId?.slice(0, 8) || "-"}
                          </td>
                          <td className="py-1 pr-3 whitespace-nowrap">
                            <span className={levelColor(log.level)}>{log.level.toUpperCase()}</span>
                          </td>
                          <td className="py-1 text-zinc-200 break-all">
                            {log.message}
                          </td>
                        </tr>
                      ))}
                      <tr><td colSpan={4}><div ref={logsEndRef} /></td></tr>
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      )}

      {/* Action confirmation dialogs */}
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
        open={deleteClusterOpen}
        title="Delete Cluster"
        message={deleteError || "PostgreSQL must be stopped before deletion. Pause the cluster first if any node is still running, then delete. This action cannot be undone."}
        confirmLabel={deleteCluster.isPending ? "Deleting..." : "Delete"}
        onConfirm={deleteClusterAfterStop}
        onCancel={() => { setDeleteClusterOpen(false); setDeleteError(null); }}
      />

      <ConfirmDialog
        open={!!actionNodeId && actionType === "restart"}
        title="Restart Node"
        message="This will stop and start PostgreSQL on the selected node. The node will be briefly unavailable. Are you sure?"
        confirmLabel="Restart"
        onConfirm={() => {
          if (actionNodeId) {
            restartNode.mutate(actionNodeId);
            setActionNodeId(null);
            setActionType(null);
          }
        }}
        onCancel={() => { setActionNodeId(null); setActionType(null); }}
      />

      <ConfirmDialog
        open={!!actionNodeId && actionType === "rejoin"}
        title="Re-sync Replica"
        message="This will repoint the replica to follow the current primary and restart PostgreSQL. Any divergent data on the replica will be overwritten. Are you sure?"
        confirmLabel="Re-sync"
        onConfirm={() => {
          if (actionNodeId) {
            rejoinNode.mutate(actionNodeId);
            setActionNodeId(null);
            setActionType(null);
          }
        }}
        onCancel={() => { setActionNodeId(null); setActionType(null); }}
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
