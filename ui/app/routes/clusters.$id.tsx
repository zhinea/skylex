import { useEffect, useRef, useState } from "react";
import { useParams, Link, useNavigate } from "react-router";
import { useCluster, useDeleteCluster, usePauseCluster, useRestartCluster, useRestartNode, useClusterHealth } from "~/hooks/useClusters";
import { useToast } from "~/components/ui/toast";
import { useNodes, useRejoinNode, useResolveInstallationConflict } from "~/hooks/useNodes";
import { type CommandLog } from "~/hooks/useCommandLogs";
import { useCommandLogStream } from "~/hooks/useCommandLogStream";
import { useConnectionProfile } from "~/hooks/useConnectionProfile";
import type { PostgresRole } from "~/hooks/usePostgresRoles";
import { Badge } from "~/components/Badge";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { PageSpinner } from "~/components/Spinner";
import { AgentStatus } from "~/components/AgentStatus";
import { LayoutDashboard, Link2, Settings as SettingsIcon, ShieldAlert, Layers, ArrowLeft, Database, Key, Shield, Lock } from "lucide-react";
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
import { ManagedRolesCard } from "~/components/cluster/ManagedRolesCard";
import { ManagedDatabasesCard } from "~/components/cluster/ManagedDatabasesCard";
import { TLSConfigCard } from "~/components/cluster/TLSConfigCard";
import { NetworkAccessCard } from "~/components/cluster/NetworkAccessCard";

type ActiveMenu = "overview" | "connection" | "databases" | "roles" | "network" | "tls" | "extensions" | "settings" | "diagnostics";

// menuItems is the built-in fallback used only when the backend does not return
// an engine module list (older server / unknown engine). Normally the sidebar is
// driven by the engine modules in the GetCluster response so engine-specific
// modules (e.g. "extensions") appear only for engines that support them.
const menuItems = [
  { id: "overview", label: "Overview", icon: LayoutDashboard },
  { id: "connection", label: "Connection", icon: Link2 },
  { id: "databases", label: "Databases", icon: Database },
  { id: "roles", label: "Roles & Users", icon: Key },
  { id: "network", label: "Network Security", icon: Shield },
  { id: "tls", label: "TLS Encryption", icon: Lock },
  { id: "settings", label: "Settings", icon: SettingsIcon },
  { id: "diagnostics", label: "Diagnostics & Logs", icon: ShieldAlert },
] as const;

type LogRange = "30m" | "1h" | "3h" | "6h" | "24h" | "7d" | "30d" | "custom";

const LOG_RANGE_PRESETS: { value: LogRange; label: string; ms: number }[] = [
  { value: "30m", label: "30 min", ms: 30 * 60 * 1000 },
  { value: "1h", label: "1 hour", ms: 60 * 60 * 1000 },
  { value: "3h", label: "3 hours", ms: 3 * 60 * 60 * 1000 },
  { value: "6h", label: "6 hours", ms: 6 * 60 * 60 * 1000 },
  { value: "24h", label: "24 hours", ms: 24 * 60 * 60 * 1000 },
  { value: "7d", label: "7 days", ms: 7 * 24 * 60 * 60 * 1000 },
  { value: "30d", label: "30 days", ms: 30 * 24 * 60 * 60 * 1000 },
  { value: "custom", label: "Custom", ms: 0 },
];

function buildLogFilter(
  range: LogRange,
  level: string,
  customSince: string,
  customUntil: string,
): { level: string; windowMs: number; sinceMs: number; untilMs: number } {
  if (range === "custom") {
    const since = customSince ? new Date(customSince).getTime() : 0;
    const until = customUntil ? new Date(customUntil).getTime() : 0;
    return {
      level,
      windowMs: 0,
      sinceMs: Number.isNaN(since) ? 0 : since,
      untilMs: Number.isNaN(until) ? 0 : until,
    };
  }
  const preset = LOG_RANGE_PRESETS.find((p) => p.value === range);
  // Pass the relative window (stable across renders); the hook materializes the
  // absolute timestamp at fetch time so the query key never churns.
  return { level, windowMs: preset ? preset.ms : 0, sinceMs: 0, untilMs: 0 };
}

function LogStreamIndicator({ state }: { state: "connecting" | "live" | "polling" | "closed" }) {
  const config = {
    connecting: { dot: "bg-amber-500 animate-pulse", label: "Connecting" },
    live: { dot: "bg-emerald-500 animate-pulse", label: "Live" },
    polling: { dot: "bg-amber-500", label: "Polling (5s)" },
    closed: { dot: "bg-zinc-500", label: "Disconnected" },
  }[state];
  return (
    <span className="ml-auto inline-flex items-center gap-1.5 text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
      <span className={`size-1.5 rounded-full ${config.dot}`} />
      {config.label}
    </span>
  );
}

export default function ClusterDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: clusterData, isLoading: clusterLoading } = useCluster(id || "");
  const { data: healthData } = useClusterHealth(id || "");
  const { data: nodesData } = useNodes(id || "");
  const [logLevel, setLogLevel] = useState<string>("");
  const [logRange, setLogRange] = useState<LogRange>("1h");
  const [customSince, setCustomSince] = useState<string>("");
  const [customUntil, setCustomUntil] = useState<string>("");
  const logFilter = buildLogFilter(logRange, logLevel, customSince, customUntil);
  const { logs, state: logStreamState } = useCommandLogStream({ clusterId: id || "", filter: logFilter });
  const { data: profileData } = useConnectionProfile(id || "");
  const startCluster = usePauseCluster(); // Note: useStartCluster mapping
  const pauseCluster = usePauseCluster();
  const restartCluster = useRestartCluster();
  const deleteCluster = useDeleteCluster();
  const toast = useToast();
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
  const [activeMenu, setActiveMenu] = useState<ActiveMenu>("overview");
  const [revealedRole, setRevealedRole] = useState<{ role: PostgresRole; password: string } | null>(null);

  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  if (clusterLoading) return <PageSpinner />;

  const cluster = clusterData?.cluster;
  if (!cluster) {
    return (
      <div className="p-8 max-w-7xl mx-auto text-center py-12">
        <h3 className="text-lg font-medium text-foreground">Cluster not found</h3>
        <Link to="/clusters" className="mt-4 text-primary hover:underline text-sm">Back to Clusters</Link>
      </div>
    );
  }

  const nodes = nodesData?.nodes || [];
  const health = healthData;
  // Build the sidebar from the engine modules returned by GetCluster so
  // engine-specific modules (e.g. extensions) only show for engines that
  // support them. Fall back to the built-in list when modules are absent.
  const sidebarItems =
    clusterData?.modules && clusterData.modules.length > 0
      ? clusterData.modules.map((m) => ({ id: m.id, label: m.label }))
      : menuItems.map((m) => ({ id: m.id as string, label: m.label }));
  // Prefer the live health snapshot for status so the badge flips from
  // "creating" to "online" without a manual refresh once provisioning settles.
  const liveStatus = health?.status ?? cluster.status;
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

  const profile = profileData?.profile;
  const effectiveHost = profileData?.effectiveHost ?? "";
  const effectivePort = profileData?.effectivePort ?? 0;
  const fallbackPrimary = nodes.find(
    (n) => n.role === "NODE_ROLE_PRIMARY" && n.postgresInstalled && n.postgresDataInitialized,
  );
  const displayHost = effectiveHost || (fallbackPrimary ? (fallbackPrimary.address || fallbackPrimary.hostname) : "");
  const displayPort = effectivePort || (fallbackPrimary?.port ?? 5432);
  const sslMode = profile?.sslMode ?? profileData?.tlsConfig?.tlsMode ?? "disabled";

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
      onSuccess: () => {
        setDeleteClusterOpen(false);
        toast.success("Cluster deleted successfully", "The cluster has been removed.");
        navigate("/clusters");
      },
      onError: (err) => {
        const message = err instanceof Error ? err.message : "Failed to delete cluster";
        setDeleteError(message.includes("running") || message.includes("pause") || message.includes("stop")
          ? `${message} Pause/stop the service first, wait for nodes to show stopped, then delete again.`
          : message);
        toast.error("Failed to delete cluster", message);
      },
    });
  }

  return (
    <div className="flex h-full w-full overflow-hidden">
      {/* Secondary Cluster Project Sidebar */}
      <aside className="w-60 flex flex-col py-4 px-3 bg-card border-r border-border h-full shrink-0">
        <div className="mb-6 px-1 space-y-3">
          <Link
            to="/clusters"
            className="flex items-center gap-1.5 text-muted-foreground hover:text-foreground transition-colors text-xs mb-4"
          >
            <span className="material-symbols-outlined text-sm">arrow_back</span>
            Back to Clusters
          </Link>
          <div className="flex flex-col gap-1">
            <div className="flex items-center justify-between gap-2">
                <h2 className="text-sm font-semibold text-foreground truncate max-w-[10rem]" title={cluster.name}>
                {cluster.name}
              </h2>
              <Badge label={liveStatus} />
            </div>
            <span className="text-[10px] font-mono text-muted-foreground">
              PostgreSQL {cluster.config?.version || "16"} • {cluster.serviceLocation === "SERVICE_LOCATION_DOCKER" || cluster.config?.serviceLocation === "SERVICE_LOCATION_DOCKER" ? "Docker" : "Native"}
            </span>
          </div>
        </div>
        <nav className="flex-1 space-y-0.5 overflow-y-auto no-scrollbar">
          {sidebarItems.map((item) => {
            const isActive = activeMenu === item.id;
            const iconName = (() => {
              switch (item.id) {
                case "overview": return "dashboard";
                case "connection": return "link";
                case "databases": return "database";
                case "roles": return "group";
                case "network": return "security";
                case "tls": return "lock";
                case "extensions": return "extension";
                case "settings": return "settings";
                case "diagnostics": return "info";
                default: return "help";
              }
            })();
            return (
              <button
                key={item.id}
                onClick={() => setActiveMenu(item.id as ActiveMenu)}
                className={`w-full flex items-center gap-2 px-2 py-1.5 rounded text-xs transition-colors cursor-pointer text-left ${
                  isActive
                    ? "bg-neutral-100 dark:bg-neutral-900 text-foreground font-medium"
                    : "text-muted-foreground hover:bg-neutral-50 dark:hover:bg-neutral-950 hover:text-foreground"
                }`}
              >
                <span className="material-symbols-outlined text-lg">{iconName}</span>
                <span>{item.label}</span>
              </button>
            );
          })}
        </nav>
        <div className="mt-auto pt-4 border-t border-border">
          <button
            onClick={() => setDeleteClusterOpen(true)}
            className="w-full flex items-center gap-2 px-2 py-1.5 rounded text-xs text-destructive hover:bg-destructive/10 transition-colors cursor-pointer text-left"
          >
            <span className="material-symbols-outlined text-lg">delete</span>
            Delete Cluster
          </button>
        </div>
      </aside>

      {/* Main Workspace Pane */}
      <main className="flex-1 overflow-y-auto scrolling-content p-6 space-y-6 bg-background">
        <div className="max-w-5xl mx-auto space-y-6">
          {activeMenu === "overview" && (
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

              {/* Redesigned Stats Grid */}
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
                    onClick={() => setActiveMenu("diagnostics")}
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
            </div>
          )}

          {activeMenu === "connection" && (
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

          {activeMenu === "databases" && (
            <div className="space-y-6">
              <ManagedDatabasesCard
                clusterId={id || ""}
                host={displayHost}
                port={displayPort}
                sslMode={sslMode}
                revealedRole={revealedRole}
              />
            </div>
          )}

          {activeMenu === "roles" && (
            <div className="space-y-6">
              <ManagedRolesCard
                clusterId={id || ""}
                host={displayHost}
                port={displayPort}
                sslMode={sslMode}
                revealed={revealedRole}
                onReveal={setRevealedRole}
                onDismissReveal={() => setRevealedRole(null)}
              />
            </div>
          )}

          {activeMenu === "network" && (
            <div className="space-y-6">
              <NetworkAccessCard clusterId={id || ""} nodes={nodes} />
            </div>
          )}

          {activeMenu === "tls" && (
            <div className="space-y-6">
              <TLSConfigCard clusterId={id || ""} nodes={nodes} />
            </div>
          )}

          {activeMenu === "settings" && (
            <div className="space-y-6">
              <SettingsCard clusterId={id || ""} cluster={cluster} />
            </div>
          )}

          {activeMenu === "diagnostics" && (
            <div className="space-y-6">
              <div className="v-card rounded-lg overflow-hidden">
                <div className="px-4 py-3 border-b border-border flex items-center gap-2 text-foreground">
                  <span className="material-symbols-outlined text-lg text-foreground">info</span>
                  <h3 className="text-xs font-semibold">Diagnostics</h3>
                </div>
                <div className="p-4 space-y-5">
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
                </div>
              </div>

              <div className="v-card rounded-lg overflow-hidden">
                <div className="px-4 py-3 border-b border-border flex items-center gap-2 text-foreground">
                  <span className="material-symbols-outlined text-lg text-foreground">terminal</span>
                  <h3 className="text-xs font-semibold">Command Logs</h3>
                  <LogStreamIndicator state={logStreamState} />
                </div>
                <div className="px-4 py-2.5 border-b border-border flex flex-wrap items-center gap-2 bg-muted/20">
                  <span className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider">Level</span>
                  <select
                    value={logLevel}
                    onChange={(e) => setLogLevel(e.target.value)}
                    className="h-7 rounded border border-border bg-background text-xs px-2 text-foreground"
                  >
                    <option value="">All</option>
                    <option value="info">Info</option>
                    <option value="warn">Warn</option>
                    <option value="error">Error</option>
                    <option value="debug">Debug</option>
                  </select>
                  <span className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider ml-2">Range</span>
                  <select
                    value={logRange}
                    onChange={(e) => setLogRange(e.target.value as LogRange)}
                    className="h-7 rounded border border-border bg-background text-xs px-2 text-foreground"
                  >
                    {LOG_RANGE_PRESETS.map((p) => (
                      <option key={p.value} value={p.value}>{p.label}</option>
                    ))}
                  </select>
                  {logRange === "custom" && (
                    <>
                      <input
                        type="datetime-local"
                        value={customSince}
                        onChange={(e) => setCustomSince(e.target.value)}
                        className="h-7 rounded border border-border bg-background text-xs px-2 text-foreground"
                        aria-label="Logs from"
                      />
                      <span className="text-[10px] text-muted-foreground">to</span>
                      <input
                        type="datetime-local"
                        value={customUntil}
                        onChange={(e) => setCustomUntil(e.target.value)}
                        className="h-7 rounded border border-border bg-background text-xs px-2 text-foreground"
                        aria-label="Logs until"
                      />
                    </>
                  )}
                  <span className="ml-auto text-[10px] text-muted-foreground">{logs.length} shown</span>
                </div>
                <div className="p-4">
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
                </div>
              </div>
            </div>
          )}
        </div>
      </main>

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
