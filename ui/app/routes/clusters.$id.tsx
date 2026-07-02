import { useState } from "react";
import { useParams, Link, useNavigate, Outlet, useOutletContext, useLocation } from "react-router";
import { useCluster, useDeleteCluster, useClusterHealth, type Cluster, type EngineModule } from "~/hooks/useClusters";
import { useToast } from "~/components/ui/toast";
import { useNodes, type Node } from "~/hooks/useNodes";
import { useConnectionProfile } from "~/hooks/useConnectionProfile";
import type { PostgresRole } from "~/hooks/usePostgresRoles";
import { Badge } from "~/components/Badge";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { PageSpinner } from "~/components/Spinner";

// menuItems is the built-in fallback used only when the backend does not return
// an engine module list (older server / unknown engine). Normally the sidebar is
// driven by the engine modules in the GetCluster response so engine-specific
// modules (e.g. "extensions") appear only for engines that support them.
const menuItems = [
  { id: "overview", label: "Overview" },
  { id: "connection", label: "Connection" },
  { id: "databases", label: "Databases" },
  { id: "roles", label: "Roles & Users" },
  { id: "network", label: "Network Security" },
  { id: "tls", label: "TLS Encryption" },
  { id: "settings", label: "Settings" },
  { id: "diagnostics", label: "Diagnostics & Logs" },
] as const;

function moduleIcon(id: string): string {
  switch (id) {
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
}

// Shared data the module pages consume via useOutletContext. Computed once in the
// layout so each page does not refetch cluster/nodes/profile independently.
export interface ClusterOutletContext {
  clusterId: string;
  cluster: Cluster;
  nodes: Node[];
  liveStatus: string;
  progressPct: number;
  onlineNodes: Node[];
  totalNodes: number;
  // Connection details derived from the connection profile / primary node, shared
  // by the connection, roles, and databases pages.
  displayHost: string;
  displayPort: number;
  sslMode: string;
  // Role reveal state is shared with the roles page so a one-time password can
  // be shown after create/rotate. The password is never displayed elsewhere —
  // the databases table renders a connection-URI template (<password>) only.
  revealedRole: { role: PostgresRole; password: string } | null;
  setRevealedRole: (value: { role: PostgresRole; password: string } | null) => void;
}

export function useClusterContext() {
  return useOutletContext<ClusterOutletContext>();
}

export default function ClusterDetailLayout() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const { data: clusterData, isLoading: clusterLoading } = useCluster(id || "");
  const { data: healthData } = useClusterHealth(id || "");
  const { data: nodesData } = useNodes(id || "");
  const { data: profileData } = useConnectionProfile(id || "");
  const deleteCluster = useDeleteCluster();
  const toast = useToast();

  const [deleteClusterOpen, setDeleteClusterOpen] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [revealedRole, setRevealedRole] = useState<{ role: PostgresRole; password: string } | null>(null);

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
  const sidebarItems: { id: string; label: string }[] =
    clusterData?.modules && clusterData.modules.length > 0
      ? (clusterData.modules as EngineModule[]).map((m) => ({ id: m.id, label: m.label }))
      : menuItems.map((m) => ({ id: m.id as string, label: m.label }));

  // Prefer the live health snapshot for status so the badge flips from
  // "creating" to "online" without a manual refresh once provisioning settles.
  const liveStatus = health?.status ?? cluster.status;
  const onlineNodes = nodes.filter((n) => n.postgresInstalled && n.postgresDataInitialized);
  const totalNodes = nodes.length;
  const progressPct = totalNodes > 0 ? Math.round((onlineNodes.length / totalNodes) * 100) : 0;

  // Connection details, shared with the connection/roles/databases pages.
  const profile = profileData?.profile;
  const effectiveHost = profileData?.effectiveHost ?? "";
  const effectivePort = profileData?.effectivePort ?? 0;
  const fallbackPrimary = nodes.find(
    (n) => n.role === "NODE_ROLE_PRIMARY" && n.postgresInstalled && n.postgresDataInitialized,
  );
  const displayHost = effectiveHost || (fallbackPrimary ? (fallbackPrimary.address || fallbackPrimary.hostname) : "");
  const displayPort = effectivePort || (fallbackPrimary?.port ?? 5432);
  const sslMode = profile?.sslMode ?? profileData?.tlsConfig?.tlsMode ?? "disabled";

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

  const ctx: ClusterOutletContext = {
    clusterId: id || "",
    cluster,
    nodes,
    liveStatus,
    progressPct,
    onlineNodes,
    totalNodes,
    displayHost,
    displayPort,
    sslMode,
    revealedRole,
    setRevealedRole,
  };

  // Determine the active module from the URL path (e.g. /clusters/:id/roles).
  const basePath = `/clusters/${id}`;
  const activeId = location.pathname.startsWith(`${basePath}/`)
    ? location.pathname.slice(`${basePath}/`.length).split("/")[0]
    : "overview";

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
            const isActive = activeId === item.id;
            return (
              <Link
                key={item.id}
                to={`${basePath}/${item.id}`}
                className={`w-full flex items-center gap-2 px-2 py-1.5 rounded text-xs transition-colors cursor-pointer text-left ${
                  isActive
                    ? "bg-neutral-100 dark:bg-neutral-900 text-foreground font-medium"
                    : "text-muted-foreground hover:bg-neutral-50 dark:hover:bg-neutral-950 hover:text-foreground"
                }`}
              >
                <span className="material-symbols-outlined text-lg">{moduleIcon(item.id)}</span>
                <span>{item.label}</span>
              </Link>
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
          <Outlet context={ctx} />
        </div>
      </main>

      <ConfirmDialog
        open={deleteClusterOpen}
        title="Delete Cluster"
        message={deleteError || "PostgreSQL must be stopped before deletion. Pause the cluster first if any node is still running, then delete. This action cannot be undone."}
        confirmLabel={deleteCluster.isPending ? "Deleting..." : "Delete"}
        onConfirm={deleteClusterAfterStop}
        onCancel={() => { setDeleteClusterOpen(false); setDeleteError(null); }}
      />
    </div>
  );
}
