import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useParams, Link } from "react-router";
import { useCluster } from "~/hooks/useClusters";
import { useNodes } from "~/hooks/useNodes";
import { useCommandLogs, type CommandLog } from "~/hooks/useCommandLogs";
import { useClusterSettings, useUpdateClusterSettings } from "~/hooks/useClusterSettings";
import { useApplyHBA, useApplyTLS, useConnectionProfile, useGenerateTLSCA, useNetworkAccess, useTLSCACert, useTLSConfig, useUpdateConnectionProfile, useUpdateNetworkAccess, useUpdateTLSConfig } from "~/hooks/useConnectionProfile";
import { useCreatePostgresDatabase, useDeletePostgresDatabase, usePostgresDatabases, type PostgresDatabase } from "~/hooks/usePostgresDatabases";
import { useCreatePostgresRole, useDeletePostgresRole, usePostgresRoles, useRotatePostgresRolePassword, type PostgresRole } from "~/hooks/usePostgresRoles";
import { useDeleteCluster, usePauseCluster, useRestartCluster, useRestartNode, useStartCluster } from "~/hooks/useClusters";
import { useRejoinNode, useResolveInstallationConflict } from "~/hooks/useNodes";
import { Badge } from "~/components/Badge";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { PageSpinner } from "~/components/Spinner";
import { SettingInput, curatedSettings, validateSettingValue } from "~/components/SettingInput";
import { AgentStatus } from "~/components/AgentStatus";
import type { Node } from "~/hooks/useNodes";
import type { Cluster } from "~/hooks/useClusters";
import { LayoutDashboard, Link2, Settings as SettingsIcon, ShieldAlert, PlusIcon, Trash2, Key, RefreshCw, Layers, Copy, ArrowLeft, Database, Network } from "lucide-react";
import { Button } from "~/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "~/components/ui/table";

function PgStatusBadges({
  installed,
  version,
  dataInitialized,
}: {
  installed: boolean;
  version: string;
  dataInitialized: boolean;
}) {
  if (!installed) {
    return <Badge label="not installed" />;
  }
  return (
    <span className="inline-flex items-center gap-1.5">
      <Badge label={version || "installed"} className="bg-emerald-50/60 text-emerald-700 border-emerald-200/50 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50" />
      <Badge
        label={dataInitialized ? "data ready" : "not initialized"}
        className={dataInitialized ? "bg-emerald-50/60 text-emerald-700 border-emerald-200/50 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50" : "bg-neutral-50 text-neutral-600 border-neutral-200 dark:bg-neutral-900/20 dark:text-neutral-400 dark:border-neutral-800"}
      />
    </span>
  );
}

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

function LifecycleCard({
  cluster,
  nodes,
  logs,
  pending,
  error,
  onAction,
}: {
  cluster: Cluster;
  nodes: Node[];
  logs: CommandLog[];
  pending: boolean;
  error: string | null;
  onAction: (action: "start" | "pause" | "restart") => void;
  }) {
  const readyNodes = nodes.filter((node) => node.agentConnected && node.postgresInstalled && node.postgresDataInitialized);
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

function levelColor(level: string): string {
  switch (level.toLowerCase()) {
    case "error":
      return "text-destructive font-semibold";
    case "warn":
      return "text-amber-600 dark:text-amber-400 font-semibold";
    case "info":
      return "text-foreground font-semibold";
    default:
      return "text-muted-foreground";
  }
}

function statusDetailColor(detail: string): string {
  switch (detail) {
    case "healthy":
    case "running":
      return "bg-emerald-50/60 text-emerald-700 border-emerald-200/50 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50 border";
    case "syncing_replica":
      return "bg-blue-50/60 text-blue-700 border-blue-200/50 dark:bg-blue-900/20 dark:text-blue-400 dark:border-blue-800/50 border";
    case "initializing_data_directory":
      return "bg-amber-50/60 text-amber-700 border-amber-200/50 dark:bg-amber-950/20 dark:text-amber-400 dark:border-amber-800/50 border";
    case "installation_conflict":
    case "waiting_for_postgres":
      return "bg-rose-50/60 text-rose-700 border-rose-200/50 dark:bg-rose-950/20 dark:text-rose-400 dark:border-rose-800/50 border";
    case "stopped":
      return "bg-neutral-50 text-neutral-600 border-neutral-200 dark:bg-neutral-900/20 dark:text-neutral-400 dark:border-neutral-800 border";
    default:
      return "bg-neutral-50 text-neutral-600 border-neutral-200 dark:bg-neutral-900/20 dark:text-neutral-400 dark:border-neutral-800 border";
  }
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <Button
      variant="outline"
      size="xs"
      onClick={handleCopy}
      title="Copy to clipboard"
      className="ml-2 h-6 px-1.5 text-[10px] uppercase font-semibold tracking-wider hover:bg-muted"
    >
      {copied ? "Copied" : "Copy"}
    </Button>
  );
}

function ConnectionRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start gap-2 py-2 border-b border-border/40 last:border-b-0">
      <dt className="w-36 shrink-0 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground pt-1">{label}</dt>
      <dd className="flex-1 min-w-0 flex items-center gap-1">
        <code className="text-xs font-mono text-foreground break-all select-all bg-muted/30 px-1.5 py-0.5 rounded border border-border/40">{value}</code>
        <CopyButton text={value} />
      </dd>
    </div>
  );
}

function FeatureNote({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="rounded-lg border border-border bg-muted/20 px-3 py-2 text-xs text-muted-foreground flex items-start gap-1.5 leading-normal">
      <span className="font-semibold text-foreground shrink-0">{title}</span>
      <span>{children}</span>
    </div>
  );
}

function RoleStatusBadge({ status }: { status: string }) {
  const normalized = (status || "pending").toLowerCase();
  let label = status || "pending";
  if (normalized === "ready") {
    return <Badge label="ready" className="bg-emerald-50/60 text-emerald-700 border-emerald-200/50 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50" />;
  }
  if (normalized === "failed") {
    return <Badge label="failed" className="bg-rose-50/60 text-rose-700 border-rose-200/50 dark:bg-rose-950/20 dark:text-rose-400 dark:border-rose-800/50" />;
  }
  if (normalized === "deleting") {
    return <Badge label="deleting" className="bg-neutral-50 text-neutral-600 border-neutral-200 dark:bg-neutral-900/20 dark:text-neutral-400 dark:border-neutral-800" />;
  }
  return <Badge label={label} className="bg-amber-50/60 text-amber-700 border-amber-200/50 dark:bg-amber-950/20 dark:text-amber-400 dark:border-amber-800/50" />;
}

function libpqSSLMode(mode: string) {
  if (mode === "disabled") return "disable";
  if (mode === "required") return "require";
  return mode || "disable";
}

function connectionURI(host: string, port: number, roleName: string, sslMode: string, password?: string) {
  const user = roleName || "<user>";
  const pass = password || "<password>";
  return `postgresql://${user}:${pass}@${host}:${port}/postgres?sslmode=${libpqSSLMode(sslMode)}`;
}

function databaseConnectionURI(host: string, port: number, databaseName: string, sslMode: string, roleName?: string, password?: string) {
  const user = roleName || "<user>";
  const pass = password || "<password>";
  return `postgresql://${user}:${pass}@${host}:${port}/${databaseName}?sslmode=${libpqSSLMode(sslMode)}`;
}

function databasePsqlCommand(host: string, port: number, databaseName: string, sslMode: string, roleName?: string) {
  return `psql "host=${host} port=${port} dbname=${databaseName} user=${roleName || "<user>"} sslmode=${libpqSSLMode(sslMode)}"`;
}

function ManagedRolesCard({
  clusterId,
  host,
  port,
  sslMode,
  revealed,
  onReveal,
  onDismissReveal,
}: {
  clusterId: string;
  host: string;
  port: number;
  sslMode: string;
  revealed: { role: PostgresRole; password: string } | null;
  onReveal: (value: { role: PostgresRole; password: string }) => void;
  onDismissReveal: () => void;
}) {
  const { data, isLoading } = usePostgresRoles(clusterId);
  const createRole = useCreatePostgresRole();
  const rotatePassword = useRotatePostgresRolePassword(clusterId);
  const deleteRole = useDeletePostgresRole(clusterId);
  const [roleName, setRoleName] = useState("");
  const [roleKind, setRoleKind] = useState("read_write");
  const [error, setError] = useState<string | null>(null);

  const roles = data?.roles ?? [];

  function handleCreate() {
    setError(null);
    createRole.mutate(
      { clusterId, roleName: roleName.trim(), roleKind },
      {
        onSuccess: (res) => {
          setRoleName("");
          onReveal({ role: res.role, password: res.oneTimePassword });
        },
        onError: (err) => setError(err instanceof Error ? err.message : "Failed to create role"),
      },
    );
  }

  function handleRotate(role: PostgresRole) {
    setError(null);
    rotatePassword.mutate(role.id, {
      onSuccess: (res) => onReveal({ role: res.role, password: res.oneTimePassword }),
      onError: (err) => setError(err instanceof Error ? err.message : "Failed to rotate password"),
    });
  }

  function handleDelete(role: PostgresRole) {
    setError(null);
    if (!window.confirm(`Drop PostgreSQL role ${role.roleName}? This cannot be undone.`)) return;
    deleteRole.mutate(role.id, {
      onError: (err) => setError(err instanceof Error ? err.message : "Failed to delete role"),
    });
  }

  const canSubmit = roleName.trim().length > 0 && !createRole.isPending;

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <Key className="size-4 text-muted-foreground" />
          Managed Roles
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        <FeatureNote title="What this does:">
          A role is a database login. Create one role per app or person so access can be rotated or removed without sharing the main PostgreSQL admin password.
        </FeatureNote>
        <p className="text-xs text-muted-foreground">
          Create application users safely. Passwords are generated by Skylex and shown only once after create or rotate.
        </p>

        {revealed && (
          <div className="rounded-lg border border-emerald-200/50 bg-emerald-50/50 p-4 dark:border-emerald-800/50 dark:bg-emerald-950/20">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0 flex-1 space-y-2">
                <p className="text-xs font-semibold uppercase tracking-wider text-emerald-800 dark:text-emerald-300">
                  One-time password generated for {revealed.role.roleName}
                </p>
                <ConnectionRow label="Password" value={revealed.password} />
                {host && <ConnectionRow label="Connection URI" value={connectionURI(host, port, revealed.role.roleName, sslMode, revealed.password)} />}
                <p className="text-[10px] font-semibold text-emerald-700 dark:text-emerald-400">
                  Save this password now. Skylex will not show it again.
                </p>
              </div>
              <Button
                variant="outline"
                size="xs"
                onClick={onDismissReveal}
                className="shrink-0 text-emerald-800 border-emerald-300/50 hover:bg-emerald-100/50 dark:text-emerald-300 dark:border-emerald-800 dark:hover:bg-emerald-900/30"
              >
                Dismiss
              </Button>
            </div>
          </div>
        )}

        <div className="rounded-lg border border-border p-4 bg-muted/10">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-[1fr_12rem_auto] md:items-end">
            <div className="space-y-1.5">
              <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Role Name</label>
              <input
                type="text"
                value={roleName}
                onChange={(e) => setRoleName(e.target.value)}
                placeholder="app_user"
                className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              />
            </div>
            <div className="space-y-1.5">
              <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Role Kind</label>
              <select
                value={roleKind}
                onChange={(e) => setRoleKind(e.target.value)}
                className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              >
                <option value="read_write" className="bg-popover text-popover-foreground">Read/Write</option>
                <option value="read_only" className="bg-popover text-popover-foreground">Read Only</option>
                <option value="admin" className="bg-popover text-popover-foreground">Admin</option>
                <option value="custom" className="bg-popover text-popover-foreground">Custom</option>
              </select>
            </div>
            <Button
              onClick={handleCreate}
              disabled={!canSubmit}
              size="sm"
            >
              {createRole.isPending ? "Creating..." : "Create Role"}
            </Button>
          </div>
          {error && <p className="mt-3 text-xs font-semibold text-destructive">{error}</p>}
        </div>

        {isLoading ? (
          <PageSpinner />
        ) : roles.length === 0 ? (
          <div className="py-8 text-center border border-dashed rounded-lg border-border/80">
            <p className="text-xs text-muted-foreground">
              No managed roles yet.
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/30">
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Role</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Kind</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Status</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Connection URI</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4 text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {roles.map((role) => (
                  <TableRow key={role.id} className="hover:bg-muted/10 border-b border-border/40 last:border-none">
                    <TableCell className="px-4 py-2.5 font-semibold text-foreground">{role.roleName}</TableCell>
                    <TableCell className="px-4 py-2.5 text-foreground/80 text-xs">{role.roleKind.replace("_", " ")}</TableCell>
                    <TableCell className="px-4 py-2.5"><RoleStatusBadge status={role.status} /></TableCell>
                    <TableCell className="px-4 py-2.5">
                      {host ? (
                        <div className="flex items-center gap-1.5">
                          <code className="max-w-[28rem] truncate text-xs text-muted-foreground font-mono bg-muted/40 px-1.5 py-0.5 rounded border border-border/40">
                            {connectionURI(host, port, role.roleName, sslMode)}
                          </code>
                          <CopyButton text={connectionURI(host, port, role.roleName, sslMode)} />
                        </div>
                      ) : (
                        <span className="text-xs text-muted-foreground">Endpoint unavailable</span>
                      )}
                    </TableCell>
                    <TableCell className="px-4 py-2.5 text-right">
                      <div className="flex justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleRotate(role)}
                          disabled={role.status === "deleting" || rotatePassword.isPending}
                          className="text-amber-600 hover:text-amber-700 hover:bg-amber-50 dark:hover:bg-amber-950/20 text-xs font-medium h-7 px-2"
                        >
                          Rotate
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleDelete(role)}
                          disabled={role.status === "deleting" || deleteRole.isPending}
                          className="text-destructive hover:text-destructive hover:bg-destructive/10 text-xs font-medium h-7 px-2"
                        >
                          Delete
                        </Button>
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
  );
}

function ManagedDatabasesCard({
  clusterId,
  host,
  port,
  sslMode,
  revealedRole,
}: {
  clusterId: string;
  host: string;
  port: number;
  sslMode: string;
  revealedRole: { role: PostgresRole; password: string } | null;
}) {
  const { data: roleData } = usePostgresRoles(clusterId);
  const { data, isLoading } = usePostgresDatabases(clusterId);
  const createDatabase = useCreatePostgresDatabase();
  const deleteDatabase = useDeletePostgresDatabase(clusterId);
  const [databaseName, setDatabaseName] = useState("");
  const [ownerRoleId, setOwnerRoleId] = useState("");
  const [error, setError] = useState<string | null>(null);

  const roles = (roleData?.roles ?? []).filter((role) => role.status === "ready" && role.roleKind !== "read_only");
  const databases = data?.databases ?? [];
  const revealedDatabasePassword = (database: PostgresDatabase) =>
    revealedRole?.role.roleName === database.ownerRoleName ? revealedRole?.password : undefined;

  function handleCreate() {
    setError(null);
    createDatabase.mutate(
      { clusterId, databaseName: databaseName.trim(), ownerRoleId: ownerRoleId || undefined },
      {
        onSuccess: () => {
          setDatabaseName("");
          setOwnerRoleId("");
        },
        onError: (err) => setError(err instanceof Error ? err.message : "Failed to create database"),
      },
    );
  }

  function handleDelete(database: PostgresDatabase) {
    setError(null);
    if (!window.confirm(`Drop PostgreSQL database ${database.databaseName}? This cannot be undone.`)) return;
    deleteDatabase.mutate(database.id, {
      onError: (err) => setError(err instanceof Error ? err.message : "Failed to delete database"),
    });
  }

  const canSubmit = databaseName.trim().length > 0 && !createDatabase.isPending;

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <Database className="size-4 text-muted-foreground" />
          Managed Databases
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        <FeatureNote title="What this does:">
          A database is where your application stores its tables and data. Pick an owner role so the app can connect with its own username and password.
        </FeatureNote>
        <p className="text-xs text-muted-foreground">
          Create application databases and optionally attach ownership to a managed read/write or admin role.
        </p>

        <div className="rounded-lg border border-border p-4 bg-muted/10">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-[1fr_12rem_auto] md:items-end">
            <div className="space-y-1.5">
              <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Database Name</label>
              <input
                type="text"
                value={databaseName}
                onChange={(e) => setDatabaseName(e.target.value)}
                placeholder="app_production"
                className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              />
            </div>
            <div className="space-y-1.5">
              <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Owner Role (optional)</label>
              <select
                value={ownerRoleId}
                onChange={(e) => setOwnerRoleId(e.target.value)}
                className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              >
                <option value="" className="bg-popover text-popover-foreground">None (admin-owned)</option>
                {roles.map((r) => (
                  <option key={r.id} value={r.id} className="bg-popover text-popover-foreground">
                    {r.roleName} ({r.roleKind.replace("_", " ")})
                  </option>
                ))}
              </select>
            </div>
            <Button
              onClick={handleCreate}
              disabled={!canSubmit}
              size="sm"
            >
              {createDatabase.isPending ? "Creating..." : "Create DB"}
            </Button>
          </div>
          {error && <p className="mt-3 text-xs font-semibold text-destructive">{error}</p>}
        </div>

        {isLoading ? (
          <PageSpinner />
        ) : databases.length === 0 ? (
          <div className="py-8 text-center border border-dashed rounded-lg border-border/80">
            <p className="text-xs text-muted-foreground">
              No databases yet.
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/30">
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Database</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Owner</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Status</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Connection URI Template</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4 text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {databases.map((database) => (
                  <TableRow key={database.id} className="hover:bg-muted/10 border-b border-border/40 last:border-none">
                    <TableCell className="px-4 py-2.5 font-semibold text-foreground">{database.databaseName}</TableCell>
                    <TableCell className="px-4 py-2.5 text-foreground/80 text-xs">{database.ownerRoleName || "Skylex admin"}</TableCell>
                    <TableCell className="px-4 py-2.5"><RoleStatusBadge status={database.status} /></TableCell>
                    <TableCell className="px-4 py-2.5">
                      {host ? (
                        <div className="space-y-1">
                          <div className="flex items-center gap-1.5">
                            <code className="max-w-[28rem] truncate text-xs text-muted-foreground font-mono bg-muted/40 px-1.5 py-0.5 rounded border border-border/40">
                              {databaseConnectionURI(
                                host,
                                port,
                                database.databaseName,
                                sslMode,
                                database.ownerRoleName,
                                revealedDatabasePassword(database),
                              )}
                            </code>
                            <CopyButton text={databaseConnectionURI(
                              host,
                              port,
                              database.databaseName,
                              sslMode,
                              database.ownerRoleName,
                              revealedDatabasePassword(database),
                            )} />
                          </div>
                          <div className="flex items-center gap-1.5">
                            <code className="max-w-[28rem] truncate text-xs text-muted-foreground/60 font-mono bg-muted/20 px-1.5 py-0.5 rounded border border-border/20">
                              {databasePsqlCommand(host, port, database.databaseName, sslMode, database.ownerRoleName)}
                            </code>
                            <CopyButton text={databasePsqlCommand(host, port, database.databaseName, sslMode, database.ownerRoleName)} />
                          </div>
                        </div>
                      ) : (
                        <span className="text-xs text-muted-foreground">Endpoint unavailable</span>
                      )}
                    </TableCell>
                    <TableCell className="px-4 py-2.5 text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDelete(database)}
                        disabled={database.status === "deleting" || deleteDatabase.isPending}
                        className="text-destructive hover:text-destructive hover:bg-destructive/10 text-xs font-medium h-7 px-2"
                      >
                        Delete
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function cidrTextToList(value: string) {
  return value
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean);
}

function cidrListToText(values: string[]) {
  return values.join("\n");
}

function NetworkAccessCard({ clusterId, nodes }: { clusterId: string; nodes: Node[] }) {
  const { data, isLoading } = useNetworkAccess(clusterId);
  const updateAccess = useUpdateNetworkAccess();
  const applyHBA = useApplyHBA();
  const [editing, setEditing] = useState(false);
  const [applicationCIDRs, setApplicationCIDRs] = useState("");
  const [adminCIDRs, setAdminCIDRs] = useState("");
  const [replicationCIDRs, setReplicationCIDRs] = useState("");
  const [message, setMessage] = useState<string | null>(null);

  const hbaStatuses = data?.hbaStatuses ?? [];
  const nodeNames = new Map(nodes.map((node) => [node.id, node.hostname]));

  function openEditor() {
    setApplicationCIDRs(cidrListToText(data?.allowedApplicationCidrs ?? []));
    setAdminCIDRs(cidrListToText(data?.allowedAdminCidrs ?? []));
    setReplicationCIDRs(cidrListToText(data?.internalReplicationCidrs ?? []));
    setMessage(null);
    setEditing(true);
  }

  function saveAccess() {
    setMessage(null);
    updateAccess.mutate(
      {
        clusterId,
        allowedApplicationCidrs: cidrTextToList(applicationCIDRs),
        allowedAdminCidrs: cidrTextToList(adminCIDRs),
        internalReplicationCidrs: cidrTextToList(replicationCIDRs),
      },
      {
        onSuccess: () => {
          setEditing(false);
          setMessage("Access rules saved. Apply rules to enforce them on nodes.");
        },
        onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to save access allowlists"),
      },
    );
  }

  function apply() {
    setMessage(null);
    applyHBA.mutate(clusterId, {
      onSuccess: () => setMessage("Access rule apply queued for ready nodes."),
      onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to queue access rule apply"),
    });
  }

  if (isLoading) {
    return (
      <Card className="shadow-xs">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground">Network Access</CardTitle>
        </CardHeader>
        <CardContent className="py-6">
          <PageSpinner />
        </CardContent>
      </Card>
    );
  }

  const renderCIDRs = (values: string[]) =>
    values.length === 0 ? (
      <span className="text-xs text-muted-foreground">No IP ranges configured</span>
    ) : (
      <div className="flex flex-wrap gap-1">
        {values.map((cidr) => (
          <code key={cidr} className="rounded border border-border bg-muted/40 px-1.5 py-0.5 font-mono text-[10px] text-foreground">
            {cidr}
          </code>
        ))}
      </div>
    );

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <Network className="size-4 text-muted-foreground" />
          Network Access
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        <FeatureNote title="What this does:">
          Decide which network ranges can connect to PostgreSQL. Skylex writes these rules into PostgreSQL's own access file, called pg_hba.conf or HBA by PostgreSQL.
        </FeatureNote>
        <p className="text-xs text-muted-foreground">
          Think of this as the database door list: if an app server's IP range is not listed here, PostgreSQL will reject it even if the port is open. Firewall and cloud security group rules are still managed outside Skylex.
        </p>

        {editing ? (
          <div className="rounded-lg border border-border p-4 bg-muted/10 space-y-4">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
              <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                App server IP ranges
                <textarea
                  value={applicationCIDRs}
                  onChange={(e) => setApplicationCIDRs(e.target.value)}
                  rows={4}
                  className="mt-1 w-full px-3 py-1.5 text-xs font-mono border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                  placeholder="10.0.0.0/24"
                />
              </label>
              <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                Admin IP ranges
                <textarea
                  value={adminCIDRs}
                  onChange={(e) => setAdminCIDRs(e.target.value)}
                  rows={4}
                  className="mt-1 w-full px-3 py-1.5 text-xs font-mono border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                  placeholder="192.168.1.0/24"
                />
              </label>
              <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                Cluster internal IP ranges
                <textarea
                  value={replicationCIDRs}
                  onChange={(e) => setReplicationCIDRs(e.target.value)}
                  rows={4}
                  className="mt-1 w-full px-3 py-1.5 text-xs font-mono border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                  placeholder="10.10.0.0/24"
                />
              </label>
            </div>
            <p className="text-[10px] text-muted-foreground leading-normal">Use one IP range per line, for example 10.0.0.0/24. If you are unsure, ask your network or cloud provider for the private CIDR range used by your application servers.</p>
            <div className="flex gap-2 pt-2 border-t border-border/40">
              <Button onClick={saveAccess} disabled={updateAccess.isPending} size="sm">
                {updateAccess.isPending ? "Saving..." : "Save Access Rules"}
              </Button>
              <Button variant="outline" onClick={() => setEditing(false)} size="sm">
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3 border border-border/60 p-4 rounded-lg bg-muted/5">
            <div>
              <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">App server IP ranges</div>
              {renderCIDRs(data?.allowedApplicationCidrs ?? [])}
            </div>
            <div>
              <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Admin IP ranges</div>
              {renderCIDRs(data?.allowedAdminCidrs ?? [])}
            </div>
            <div>
              <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Cluster internal IP ranges</div>
              {renderCIDRs(data?.internalReplicationCidrs ?? [])}
            </div>
          </div>
        )}

        <div className="flex flex-wrap items-center gap-2.5">
          {!editing && (
            <Button onClick={openEditor} variant="outline" size="sm">
              Edit Access Rules
            </Button>
          )}
          <Button onClick={apply} disabled={applyHBA.isPending} size="sm">
            {applyHBA.isPending ? "Queueing..." : "Apply Access Rules"}
          </Button>
          {message && <span className="text-xs font-medium text-muted-foreground">{message}</span>}
        </div>

        <div className="space-y-2.5">
          <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Access Rule Apply Status</div>
          {hbaStatuses.length === 0 ? (
            <div className="py-6 text-center border border-dashed rounded-lg border-border/80">
              <p className="text-xs text-muted-foreground">Access rules have not been applied yet.</p>
            </div>
          ) : (
            <div className="overflow-x-auto rounded-lg border border-border">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/30">
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Node</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Status</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Updated</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Error</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {hbaStatuses.map((status) => (
                    <TableRow key={status.nodeId} className="hover:bg-muted/10 border-b border-border/40 last:border-none">
                      <TableCell className="px-4 py-2.5 font-semibold text-foreground">{nodeNames.get(status.nodeId) || status.nodeId.slice(0, 8)}</TableCell>
                      <TableCell className="px-4 py-2.5"><RoleStatusBadge status={status.status} /></TableCell>
                      <TableCell className="text-muted-foreground px-4 py-2.5 text-xs">{status.updatedAt ? new Date(status.updatedAt).toLocaleString() : "-"}</TableCell>
                      <TableCell className="text-destructive px-4 py-2.5 text-xs font-semibold">{status.error || "-"}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function TLSConfigCard({ clusterId, nodes }: { clusterId: string; nodes: Node[] }) {
  const { data, isLoading } = useTLSConfig(clusterId);
  const updateTLS = useUpdateTLSConfig();
  const generateCA = useGenerateTLSCA();
  const applyTLS = useApplyTLS();
  const [editing, setEditing] = useState(false);
  const [tlsMode, setTLSMode] = useState("disabled");
  const [certFile, setCertFile] = useState("");
  const [keyFile, setKeyFile] = useState("");
  const [caFile, setCAFile] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const [generatedCAPem, setGeneratedCAPem] = useState<string | null>(null);

  const config = data?.config;
  const { data: caData } = useTLSCACert(clusterId, !!config?.caGenerated);
  const caCertPem = generatedCAPem ?? caData?.caCertPem ?? "";
  const statuses = config?.statuses ?? [];
  const warnings = config?.warnings ?? [];
  const nodeNames = new Map(nodes.map((node) => [node.id, node.hostname]));

  function openEditor() {
    setTLSMode(config?.tlsMode ?? "disabled");
    setCertFile(config?.certFile ?? "");
    setKeyFile(config?.keyFile ?? "");
    setCAFile(config?.caFile ?? "");
    setMessage(null);
    setEditing(true);
  }

  function saveTLS() {
    setMessage(null);
    updateTLS.mutate(
      { clusterId, tlsMode, certFile, keyFile, caFile },
      {
        onSuccess: () => {
          setEditing(false);
          setMessage("TLS configuration saved. Apply TLS to enforce it on ready nodes.");
        },
        onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to save TLS configuration"),
      },
    );
  }

  function apply() {
    setMessage(null);
    applyTLS.mutate(clusterId, {
      onSuccess: () => setMessage("TLS apply queued for ready nodes."),
      onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to queue TLS apply"),
    });
  }

  function generate() {
    setMessage(null);
    const continueAnyway = window.confirm(
      "This action will generate or replace the cluster PostgreSQL TLS CA, save TLS mode as prefer, queue TLS apply commands for ready nodes, and create a CA certificate download link. Existing clients that trust the previous CA may need the new certificate. Continue anyway?",
    );
    if (!continueAnyway) return;
    generateCA.mutate(clusterId, {
      onSuccess: (res) => {
        setGeneratedCAPem(res.caCertPem);
        updateTLS.mutate(
          { clusterId, tlsMode: "prefer", certFile: "", keyFile: "", caFile: "" },
          {
            onSuccess: () => {
              applyTLS.mutate(clusterId, {
                onSuccess: () => setMessage("Cluster TLS CA generated, TLS apply queued, and CA certificate is ready to download."),
                onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to queue TLS apply"),
              });
            },
            onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to save TLS configuration"),
          },
        );
      },
      onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to generate TLS CA"),
    });
  }

  function downloadCA() {
    const pem = caCertPem;
    if (!pem || typeof window === "undefined") return;
    const url = window.URL.createObjectURL(new Blob([pem], { type: "application/x-pem-file" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = `skylex-${clusterId}-ca.crt`;
    link.click();
    window.URL.revokeObjectURL(url);
  }

  if (isLoading) {
    return (
      <Card className="shadow-xs">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground">TLS</CardTitle>
        </CardHeader>
        <CardContent className="py-6">
          <PageSpinner />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <ShieldAlert className="size-4 text-muted-foreground" />
          TLS Configuration
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        <FeatureNote title="What this does:">
          TLS encrypts database traffic between apps and PostgreSQL, like HTTPS for your database. It is off by default; generate a CA certificate when you are ready to let clients trust Skylex-managed database certificates.
        </FeatureNote>
        <p className="text-xs text-muted-foreground">
          Beginner path: click Generate CA Cert, confirm, then download the CA certificate and give it to applications that require trusted encrypted database connections. Use manual certificate paths only if your team already manages PostgreSQL certificates outside Skylex.
        </p>

        {warnings.length > 0 && (
          <div className="space-y-2">
            {warnings.map((warning) => (
              <div key={warning} className="rounded-lg border border-amber-200/50 bg-amber-50/50 px-3 py-2 text-xs text-amber-800 dark:border-amber-800/40 dark:bg-amber-950/20 dark:text-amber-300 font-medium">
                {warning}
              </div>
            ))}
          </div>
        )}

        {editing ? (
          <div className="rounded-lg border border-border p-4 bg-muted/10 space-y-4">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">TLS Mode</label>
                <select
                  value={tlsMode}
                  onChange={(e) => setTLSMode(e.target.value)}
                  className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                >
                  <option value="disabled" className="bg-popover text-popover-foreground">disabled</option>
                  <option value="prefer" className="bg-popover text-popover-foreground">prefer</option>
                  <option value="required" className="bg-popover text-popover-foreground">required</option>
                </select>
                <p className="text-[10px] text-muted-foreground leading-normal">disabled = no encryption required, prefer = allow encrypted clients, required = reject unencrypted clients.</p>
              </div>
              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Server Certificate Path (optional)</label>
                <input
                  type="text"
                  value={certFile}
                  onChange={(e) => setCertFile(e.target.value)}
                  placeholder="/etc/skylex/postgres/server.crt"
                  className="w-full px-3 py-1.5 text-xs font-mono border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                />
              </div>
              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Server Key Path (optional)</label>
                <input
                  type="text"
                  value={keyFile}
                  onChange={(e) => setKeyFile(e.target.value)}
                  placeholder="/etc/skylex/postgres/server.key"
                  className="w-full px-3 py-1.5 text-xs font-mono border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                />
              </div>
              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">CA File Path (optional)</label>
                <input
                  type="text"
                  value={caFile}
                  onChange={(e) => setCAFile(e.target.value)}
                  placeholder="/etc/skylex/postgres/ca.crt"
                  className="w-full px-3 py-1.5 text-xs font-mono border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                />
              </div>
            </div>
            <p className="text-[10px] text-muted-foreground leading-normal">Leave certificate and key paths empty unless you already have your own PostgreSQL certificate files on every node. Empty paths tell Skylex to generate and install certificates for you.</p>
            <div className="flex gap-2 pt-2 border-t border-border/40">
              <Button onClick={saveTLS} disabled={updateTLS.isPending} size="sm">
                {updateTLS.isPending ? "Saving..." : "Save TLS"}
              </Button>
              <Button variant="outline" onClick={() => setEditing(false)} size="sm">
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <dl className="rounded-lg border border-border/60 px-3 bg-muted/5">
            <ConnectionRow label="TLS Mode" value={config?.tlsMode ?? "disabled"} />
            <ConnectionRow label="Cluster CA" value={config?.caGenerated ? "Generated" : "Not generated"} />
            <ConnectionRow label="Server Certificate" value={config?.certFile || ((config?.tlsMode ?? "disabled") === "disabled" ? "Not configured" : "Skylex-managed CA-signed per node")} />
            <ConnectionRow label="Server Key" value={config?.keyFile || ((config?.tlsMode ?? "disabled") === "disabled" ? "Not configured" : "Skylex-managed CA-signed per node")} />
            <ConnectionRow label="CA File" value={config?.caFile || ((config?.tlsMode ?? "disabled") !== "disabled" && config?.caGenerated ? "Skylex-managed per node" : "Not configured")} />
          </dl>
        )}

        <div className="flex flex-wrap items-center gap-2.5">
          {!editing && (
            <Button onClick={openEditor} variant="outline" size="sm">
              Edit TLS
            </Button>
          )}
          <Button onClick={generate} disabled={generateCA.isPending || updateTLS.isPending || applyTLS.isPending} variant="outline" size="sm">
            {generateCA.isPending || updateTLS.isPending || applyTLS.isPending ? "Configuring..." : config?.caGenerated ? "Regenerate CA Cert" : "Generate CA Cert"}
          </Button>
          {(config?.caGenerated || generatedCAPem) && (
            <Button onClick={downloadCA} disabled={!caCertPem} variant="outline" size="sm">
              Download CA Cert
            </Button>
          )}
          <Button onClick={apply} disabled={applyTLS.isPending || ((config?.tlsMode ?? "disabled") !== "disabled" && !config?.caGenerated && !config?.certFile)} size="sm">
            {applyTLS.isPending ? "Queueing..." : "Apply TLS"}
          </Button>
          {message && <span className="text-xs font-medium text-muted-foreground">{message}</span>}
        </div>

        <div className="space-y-2.5">
          <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">TLS Apply Status</div>
          {statuses.length === 0 ? (
            <div className="py-6 text-center border border-dashed rounded-lg border-border/80">
              <p className="text-xs text-muted-foreground">TLS has not been applied yet.</p>
            </div>
          ) : (
            <div className="overflow-x-auto rounded-lg border border-border">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/30">
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Node</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Mode</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Status</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Active</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Updated</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4 text-right">Error</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {statuses.map((status) => (
                    <TableRow key={status.nodeId} className="hover:bg-muted/10 border-b border-border/40 last:border-none">
                      <TableCell className="px-4 py-2.5 font-semibold text-foreground">{nodeNames.get(status.nodeId) || status.nodeId.slice(0, 8)}</TableCell>
                      <TableCell className="px-4 py-2.5 text-muted-foreground text-xs font-mono">{status.requestedTlsMode}</TableCell>
                      <TableCell className="px-4 py-2.5"><RoleStatusBadge status={status.status} /></TableCell>
                      <TableCell className="px-4 py-2.5 text-muted-foreground text-xs font-semibold">{status.tlsActive ? "yes" : "no"}</TableCell>
                      <TableCell className="px-4 py-2.5 text-muted-foreground text-xs">{status.updatedAt ? new Date(status.updatedAt).toLocaleString() : "-"}</TableCell>
                      <TableCell className="text-destructive px-4 py-2.5 text-xs font-semibold text-right">{status.error || "-"}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function PostgreSQLConnectionCard({ clusterId, nodes, cluster }: { clusterId: string; nodes: Node[]; cluster: Cluster }) {
  // Only show when nodes are assigned
  if (nodes.length === 0) return null;

  const { data: profileData, isLoading: profileLoading } = useConnectionProfile(clusterId);
  const updateProfile = useUpdateConnectionProfile();

  // Edit state
  const [editing, setEditing] = useState(false);
  const [editMode, setEditMode] = useState("direct_primary");
  const [editPublicHost, setEditPublicHost] = useState("");
  const [editPublicPort, setEditPublicPort] = useState(5432);
  const [editSSLMode, setEditSSLMode] = useState("disabled");
  const [editCIDRs, setEditCIDRs] = useState("");
  const [saveError, setSaveError] = useState<string | null>(null);
  const [savedOk, setSavedOk] = useState(false);
  const [revealedRole, setRevealedRole] = useState<{ role: PostgresRole; password: string } | null>(null);

  const profile = profileData?.profile;
  const primaryEndpoint = profileData?.primaryEndpoint;
  const replicaEndpoints = profileData?.replicaEndpoints ?? [];
  const effectiveHost = profileData?.effectiveHost ?? "";
  const effectivePort = profileData?.effectivePort ?? 0;

  // Fallback to node data when profile API hasn't loaded yet
  const fallbackPrimary = nodes.find(
    (n) => n.role === "NODE_ROLE_PRIMARY" && n.postgresInstalled && n.postgresDataInitialized,
  );
  const fallbackReplicas = nodes.filter(
    (n) => n.role === "NODE_ROLE_REPLICA" && n.postgresInstalled && n.postgresDataInitialized,
  );

  const serviceLocation =
    cluster.serviceLocation === "SERVICE_LOCATION_DOCKER" ||
    cluster.config?.serviceLocation === "SERVICE_LOCATION_DOCKER"
      ? "Dockerized"
      : "Native";

  // Determine the endpoint to display
  const displayHost = effectiveHost || (fallbackPrimary ? (fallbackPrimary.address || fallbackPrimary.hostname) : "");
  const displayPort = effectivePort || (fallbackPrimary?.port ?? 5432);
  const sslMode = profile?.sslMode ?? profileData?.tlsConfig?.tlsMode ?? "disabled";
  const connectionSSLMode = libpqSSLMode(sslMode);
  const profileWarnings = profileData?.warnings ?? profileData?.tlsConfig?.warnings ?? [];

  const isPrimaryReady = !!primaryEndpoint || !!fallbackPrimary;

  function handleEditOpen() {
    setEditMode(profile?.endpointMode ?? "direct_primary");
    setEditPublicHost(profile?.publicHost ?? "");
    setEditPublicPort(profile?.publicPort ?? 5432);
    setEditSSLMode(profile?.sslMode ?? "disabled");
    setEditCIDRs((profile?.allowedCidrs ?? []).join(", "));
    setSaveError(null);
    setSavedOk(false);
    setEditing(true);
  }

  function handleSave() {
    setSaveError(null);
    const cidrs = editCIDRs
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    updateProfile.mutate(
      {
        clusterId,
        endpointMode: editMode,
        publicHost: editPublicHost,
        publicPort: editPublicPort,
        sslMode: editSSLMode,
        allowedCidrs: cidrs,
      },
      {
        onSuccess: () => {
          setSavedOk(true);
          setEditing(false);
          setTimeout(() => setSavedOk(false), 3000);
        },
        onError: (err) => {
          setSaveError(err instanceof Error ? err.message : "Failed to save profile");
        },
      },
    );
  }

  if (profileLoading) {
    return (
      <Card className="shadow-xs">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
            <Database className="size-4 text-muted-foreground" />
            PostgreSQL Connection
          </CardTitle>
        </CardHeader>
        <CardContent className="py-6">
          <PageSpinner />
        </CardContent>
      </Card>
    );
  }

  if (!isPrimaryReady) {
    return (
      <Card className="shadow-xs border-amber-200/50 dark:border-amber-800/40">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
            <Database className="size-4 text-muted-foreground" />
            PostgreSQL Connection
          </CardTitle>
        </CardHeader>
        <CardContent className="pt-6">
          <div className="flex items-start gap-3 rounded-lg border border-amber-200/50 bg-amber-50/50 px-4 py-3 dark:border-amber-800/40 dark:bg-amber-950/20">
            <span className="text-amber-600 dark:text-amber-400 mt-0.5 text-base leading-none">⏳</span>
            <div className="text-xs">
              <p className="font-semibold text-amber-900 dark:text-amber-100">Primary not ready</p>
              <p className="mt-1 text-muted-foreground leading-normal">
                Connection details will appear once the primary node has PostgreSQL installed and data
                initialized.
              </p>
            </div>
          </div>
        </CardContent>
      </Card>
    );
  }

  const endpoint = displayHost ? `${displayHost}:${displayPort}` : "";
  const psqlCommand = displayHost
    ? `psql "host=${displayHost} port=${displayPort} dbname=postgres user=<user> sslmode=${connectionSSLMode}"`
    : "";
  const uriTemplate = displayHost
    ? `postgresql://<user>:<password>@${displayHost}:${displayPort}/postgres?sslmode=${connectionSSLMode}`
    : "";

  const isManualMode = profile?.endpointMode === "manual_stable_endpoint";
  const hasPublicHost = !!(profile?.publicHost);

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4 flex flex-row items-center justify-between space-y-0">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <Database className="size-4 text-muted-foreground" />
          PostgreSQL Connection
        </CardTitle>
        <div className="flex items-center gap-2">
          {savedOk && (
            <span className="text-xs font-medium text-emerald-600 dark:text-emerald-400">Profile saved.</span>
          )}
          {!editing && (
            <Button
              onClick={handleEditOpen}
              variant="outline"
              size="xs"
            >
              Edit Profile
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-6 pt-6">
        <FeatureNote title="What this does:">
          Use this section to copy the address that applications need to reach PostgreSQL. The generated examples already include the current encryption setting and the default database name.
        </FeatureNote>

        {/* Edit form */}
        {editing && (
          <div className="rounded-lg border border-border p-4 bg-muted/10 space-y-4">
            <div className="space-y-1">
              <p className="text-xs font-semibold text-foreground">Connection Profile Settings</p>
              <p className="text-[11px] text-muted-foreground leading-normal">
                Direct Primary is easiest for testing. For production apps, use Manual Stable Endpoint with a DNS name, load balancer, or virtual IP that does not change after failover.
              </p>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Endpoint Mode</label>
                <select
                  value={editMode}
                  onChange={(e) => setEditMode(e.target.value)}
                  className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                >
                  <option value="direct_primary" className="bg-popover text-popover-foreground">Direct Primary (computed from node)</option>
                  <option value="manual_stable_endpoint" className="bg-popover text-popover-foreground">Manual Stable Endpoint</option>
                </select>
              </div>

              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Connection encryption</label>
                <select
                  value={editSSLMode}
                  onChange={(e) => setEditSSLMode(e.target.value)}
                  className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                >
                  <option value="disabled" className="bg-popover text-popover-foreground">disabled</option>
                  <option value="prefer" className="bg-popover text-popover-foreground">prefer</option>
                  <option value="required" className="bg-popover text-popover-foreground">required</option>
                </select>
                <p className="text-[10px] text-muted-foreground leading-normal">disabled maps to sslmode=disable, prefer maps to sslmode=prefer, and required maps to require.</p>
              </div>

              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Public Host</label>
                <input
                  type="text"
                  value={editPublicHost}
                  onChange={(e) => setEditPublicHost(e.target.value)}
                  placeholder="pg.example.com"
                  className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                />
              </div>

              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Public Port</label>
                <input
                  type="number"
                  min={1}
                  max={65535}
                  value={editPublicPort}
                  onChange={(e) => setEditPublicPort(Number(e.target.value))}
                  className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                />
              </div>

              <div className="md:col-span-2 space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  Application IP ranges <span className="text-muted-foreground font-normal">(comma-separated)</span>
                </label>
                <input
                  type="text"
                  value={editCIDRs}
                  onChange={(e) => setEditCIDRs(e.target.value)}
                  placeholder="10.0.0.0/8, 0.0.0.0/0"
                  className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                />
                <p className="text-[10px] text-muted-foreground leading-normal">These ranges are also used as app connection allowlists. 0.0.0.0/0 means any IPv4 address and is not recommended for production.</p>
              </div>
            </div>

            {saveError && (
              <p className="text-xs font-semibold text-destructive">{saveError}</p>
            )}

            <div className="flex gap-2 pt-2 border-t border-border/40">
              <Button
                onClick={handleSave}
                disabled={updateProfile.isPending}
                size="sm"
              >
                {updateProfile.isPending ? "Saving..." : "Save Settings"}
              </Button>
              <Button
                variant="outline"
                onClick={() => setEditing(false)}
                size="sm"
              >
                Cancel
              </Button>
            </div>
          </div>
        )}

        {/* Warnings */}
        <div className="space-y-2">
          {!isManualMode && (
            <div className="flex items-start gap-2.5 rounded-lg border border-amber-200/50 bg-amber-50/50 px-3 py-2 dark:border-amber-800/40 dark:bg-amber-950/20 text-xs text-amber-800 dark:text-amber-300 font-medium">
              <span className="mt-0.5 shrink-0 text-amber-600 dark:text-amber-400">⚠️</span>
              <p>
                <span className="font-semibold text-foreground">Direct node endpoint:</span> This endpoint points directly
                to the primary node and may change after a failover. Configure a stable endpoint (VIP, DNS, or
                proxy) via Edit Profile.
              </p>
            </div>
          )}
          {isManualMode && !hasPublicHost && (
            <div className="flex items-start gap-2.5 rounded-lg border border-amber-200/50 bg-amber-50/50 px-3 py-2 dark:border-amber-800/40 dark:bg-amber-950/20 text-xs text-amber-800 dark:text-amber-300 font-medium">
              <span className="mt-0.5 shrink-0 text-amber-600 dark:text-amber-400">⚠️</span>
              <p>
                <span className="font-semibold text-foreground">Manual stable endpoint mode is active</span> but no public host is
                set. Set a public host via Edit Profile or switch to Direct Primary.
              </p>
            </div>
          )}
          <div className="flex items-start gap-2.5 rounded-lg border border-border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
            <span className="mt-0.5 shrink-0 text-muted-foreground">ℹ️</span>
            <p>
              <span className="font-semibold text-foreground">Firewall:</span> Ensure port {displayPort} is open from your
              application to the PostgreSQL node. Skylex does not manage firewall or security group
              rules.
            </p>
          </div>
          {profileWarnings.map((warning) => (
            <div key={warning} className="flex items-start gap-2.5 rounded-lg border border-amber-200/50 bg-amber-50/50 px-3 py-2 dark:border-amber-800/40 dark:bg-amber-950/20 text-xs text-amber-800 dark:text-amber-300 font-medium">
              <span className="mt-0.5 shrink-0 text-amber-600 dark:text-amber-400">⚠️</span>
              <p className="text-xs font-semibold text-amber-800 dark:text-amber-300">{warning}</p>
            </div>
          ))}
        </div>

        {/* Connection details */}
        {endpoint && (
          <dl className="rounded-lg border border-border bg-muted/5">
            <ConnectionRow label="Primary Endpoint" value={endpoint} />
            <ConnectionRow label="Default Database" value="postgres" />
            <ConnectionRow label="Connection encryption" value={sslMode} />
            <ConnectionRow label="Service Location" value={serviceLocation} />
            {psqlCommand && <ConnectionRow label="psql Command" value={psqlCommand} />}
            {uriTemplate && <ConnectionRow label="URI Template" value={uriTemplate} />}
          </dl>
        )}

        {endpoint && (
          <p className="text-[11px] text-muted-foreground leading-normal">
            Replace <code className="font-mono text-foreground bg-muted/40 px-1 rounded">&lt;user&gt;</code> and{" "}
            <code className="font-mono text-foreground bg-muted/40 px-1 rounded">&lt;password&gt;</code> with your PostgreSQL credentials.
            Stored passwords are never displayed; generated passwords appear only once after create or rotate.
          </p>
        )}

        <ManagedRolesCard
          clusterId={clusterId}
          host={displayHost}
          port={displayPort}
          sslMode={sslMode}
          revealed={revealedRole}
          onReveal={setRevealedRole}
          onDismissReveal={() => setRevealedRole(null)}
        />
        <ManagedDatabasesCard clusterId={clusterId} host={displayHost} port={displayPort} sslMode={sslMode} revealedRole={revealedRole} />
        <TLSConfigCard clusterId={clusterId} nodes={nodes} />
        <NetworkAccessCard clusterId={clusterId} nodes={nodes} />

        {/* Replica endpoints */}
        {(replicaEndpoints.length > 0 || fallbackReplicas.length > 0) && (
          <div className="space-y-2.5">
            <div className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider">
              Replica Endpoints (read-only)
            </div>
            <dl className="rounded-lg border border-border bg-muted/5">
              {(replicaEndpoints.length > 0 ? replicaEndpoints : fallbackReplicas.map((n) => ({
                nodeId: n.id,
                hostname: n.hostname,
                host: n.address || n.hostname,
                port: n.port || 5432,
                role: n.role,
              }))).map((ep) => (
                <ConnectionRow
                  key={ep.nodeId}
                  label={ep.hostname}
                  value={`${ep.host}:${ep.port}`}
                />
              ))}
            </dl>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function InstallationConflictCard({ nodes, onResolve, pending }: {
  nodes: Node[];
  onResolve: (nodeId: string, action: "ADOPT" | "PURGE" | "ABORT") => void;
  pending: boolean;
}) {
  if (nodes.length === 0) return null;

  return (
    <Card className="border-destructive/30 shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-destructive flex items-center gap-2">
          <ShieldAlert className="size-4 text-destructive" />
          Native PostgreSQL Conflict
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        <div className="rounded-lg border border-destructive/20 bg-destructive/10 px-4 py-3 text-xs leading-relaxed text-destructive/90">
          <p className="font-semibold text-destructive mb-1">
            Existing native PostgreSQL or data was found on {nodes.length} selected node{nodes.length === 1 ? "" : "s"}.
          </p>
          <p>
            Skylex is paused to avoid unplanned data loss. Choose <strong className="text-foreground">Use Existing</strong> to adopt the detected installation, <strong className="text-foreground">Remove & Reinstall</strong> to purge packages and the configured data directory, or <strong className="text-foreground">Abort Cluster Creation</strong>.
          </p>
        </div>

        {nodes.map((node) => (
          <div key={node.id} className="rounded-lg border border-border p-4 bg-muted/5 space-y-4">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
              <div className="space-y-1">
                <div className="font-semibold text-foreground">{node.hostname}</div>
                <div className="text-xs text-muted-foreground font-mono bg-muted/40 px-2 py-1 rounded border border-border/40">
                  {node.conflictDetails || "Existing PostgreSQL installation or data directory content detected."}
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  onClick={() => onResolve(node.id, "ADOPT")}
                  disabled={pending}
                  size="sm"
                >
                  Use Existing
                </Button>
                <Button
                  onClick={() => onResolve(node.id, "PURGE")}
                  disabled={pending}
                  variant="destructive"
                  size="sm"
                >
                  Remove & Reinstall
                </Button>
                <Button
                  onClick={() => onResolve(node.id, "ABORT")}
                  disabled={pending}
                  variant="outline"
                  size="sm"
                >
                  Abort Cluster Creation
                </Button>
              </div>
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function SettingsCard({ clusterId }: { clusterId: string }) {
  const { data, isLoading } = useClusterSettings(clusterId);
  const update = useUpdateClusterSettings();
  const [values, setValues] = useState<Record<string, string>>({});
  const [errors, setErrors] = useState<Record<string, string | null>>({});
  const [saved, setSaved] = useState(false);

  const parameters = useMemo(() => data?.settings?.parameters ?? {}, [data?.settings?.parameters]);

  useEffect(() => {
    const next: Record<string, string> = {};
    for (const s of curatedSettings) {
      next[s.key] = parameters[s.key] ?? "";
    }
    setValues(next);
  }, [parameters]);

  const dirty = useMemo(() => {
    let changed = false;
    for (const s of curatedSettings) {
      if ((values[s.key] ?? "") !== (parameters[s.key] ?? "")) {
        changed = true;
      }
    }
    return changed;
  }, [values, parameters]);

  function handleChange(key: string, value: string) {
    setValues((prev) => ({ ...prev, [key]: value }));
    setErrors((prev) => ({ ...prev, [key]: validateSettingValue(key, value) }));
    setSaved(false);
  }

  function handleSave() {
    const nextErrors: Record<string, string | null> = {};
    const payload: Record<string, string> = {};

    for (const s of curatedSettings) {
      const v = values[s.key]?.trim() ?? "";
      const err = validateSettingValue(s.key, v);
      nextErrors[s.key] = err;
      if (!err && v) {
        payload[s.key] = v;
      }
    }

    setErrors(nextErrors);
    if (Object.values(nextErrors).some(Boolean)) {
      return;
    }

    update.mutate(
      { clusterId, settings: payload },
      {
        onSuccess: () => {
          setSaved(true);
          setTimeout(() => setSaved(false), 3000);
        },
      },
    );
  }

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <SettingsIcon className="size-4 text-muted-foreground" />
          PostgreSQL Settings
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        {isLoading ? (
          <PageSpinner />
        ) : (
          <div className="space-y-5">
            <FeatureNote title="What this does:">
              Tune common PostgreSQL behavior without editing configuration files by hand. Skylex validates values, saves them, and queues the needed reload or restart on cluster nodes.
            </FeatureNote>
            <p className="text-xs text-muted-foreground">
              Start with defaults unless you know a setting solves a specific workload problem. Some settings apply with a quick reload; others need PostgreSQL to restart.
            </p>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {curatedSettings.map((s) => (
                <SettingInput
                  key={s.key}
                  id={s.key}
                  label={s.label}
                  type={s.type}
                  value={values[s.key] ?? ""}
                  onChange={(v) => handleChange(s.key, v)}
                  hint={s.hint}
                  options={s.options}
                  disabled={update.isPending}
                />
              ))}
            </div>
            {curatedSettings.map((s) =>
              errors[s.key] ? (
                <p key={`${s.key}-err`} className="text-xs font-semibold text-destructive">
                  {s.label}: {errors[s.key]}
                </p>
              ) : null,
            )}
            <div className="flex items-center gap-3 pt-2">
              <Button
                onClick={handleSave}
                disabled={!dirty || update.isPending}
                size="sm"
              >
                {update.isPending ? "Saving..." : "Apply Settings"}
              </Button>
              {saved && (
                <span className="text-xs font-semibold text-emerald-600 dark:text-emerald-400">Settings queued for all nodes.</span>
              )}
              {update.isError && (
                <span className="text-xs font-semibold text-destructive">
                  {update.error instanceof Error ? update.error.message : "Failed to update settings"}
                </span>
              )}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function InstallationProgressCard({ nodes, logs }: { nodes: Node[]; logs: CommandLog[] }) {
  const nodeList = nodes;
  const totalNodes = nodeList.length;
  const readyNodes = nodeList.filter((n) => n.postgresInstalled && n.postgresDataInitialized);
  const progressPct = totalNodes > 0 ? Math.round((readyNodes.length / totalNodes) * 100) : 0;
  const tailLogs = logs.slice(-12);

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <RefreshCw className="size-4 text-muted-foreground animate-spin-slow" />
          Installation Progress
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        <FeatureNote title="What this does:">
          Track the setup steps Skylex runs on each node, such as installing PostgreSQL, initializing data, starting the service, and preparing replication.
        </FeatureNote>
        <div>
          <div className="flex justify-between text-xs mb-2">
            <span className="font-semibold text-muted-foreground uppercase tracking-wider">Provisioning</span>
            <span className="text-foreground font-semibold">
              {readyNodes.length}/{totalNodes} nodes ready
            </span>
          </div>
          <div className="w-full bg-muted rounded-full h-2 overflow-hidden border border-border/50">
            <div
              className={`h-2 rounded-full transition-all duration-500 ${
                progressPct === 100 ? "bg-emerald-500" : progressPct > 0 ? "bg-primary" : "bg-amber-500"
              }`}
              style={{ width: `${progressPct}%` }}
            />
          </div>
        </div>

        {nodeList.length > 0 && (
          <div className="overflow-x-auto rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/30">
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Node</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Location</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Install State</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {nodeList.map((n) => (
                  <TableRow key={n.id} className="hover:bg-muted/10 border-b border-border/40 last:border-none">
                    <TableCell className="px-4 py-2.5">
                      <div className="text-foreground font-semibold">{n.hostname}</div>
                      <div className="text-[10px] text-muted-foreground uppercase tracking-wider font-mono">{n.role}</div>
                    </TableCell>
                    <TableCell className="px-4 py-2.5 text-foreground/80 text-xs">
                      {n.serviceLocation === "SERVICE_LOCATION_DOCKER" ? "Dockerized" : "Native"}
                    </TableCell>
                    <TableCell className="px-4 py-2.5">
                      <PgStatusBadges
                        installed={n.postgresInstalled}
                        version={n.postgresVersion}
                        dataInitialized={n.postgresDataInitialized}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        <div className="space-y-2">
          <div className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider">
            Recent Installation Logs
          </div>
          {tailLogs.length === 0 ? (
            <p className="text-xs text-muted-foreground">Logs appear once agents start executing installation commands.</p>
          ) : (
            <div className="max-h-48 overflow-y-auto font-mono text-[11px] rounded-lg bg-zinc-950 text-zinc-200 border border-zinc-800 p-3 space-y-1">
              {tailLogs.map((log) => (
                <div key={log.id} className="grid grid-cols-[5rem_7rem_1fr] gap-2 py-0.5 border-b border-zinc-900/50 last:border-b-0 leading-relaxed">
                  <span className="text-zinc-500">{new Date(Number(log.timestampMs)).toLocaleTimeString()}</span>
                  <span className="text-zinc-400 truncate">{log.hostname || log.nodeId?.slice(0, 8) || "-"}</span>
                  <span className={`${levelColor(log.level)} break-all`}>{log.message}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

export default function ClusterDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: clusterData, isLoading: clusterLoading } = useCluster(id || "");
  const { data: nodesData } = useNodes(id || "");
  const { data: logsData } = useCommandLogs(id || "");
  const startCluster = useStartCluster();
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
        <h3 className="text-lg font-medium text-gray-900 dark:text-white">Cluster not found</h3>
        <Link to="/clusters" className="mt-4 text-blue-600 hover:text-blue-800 text-sm">Back to Clusters</Link>
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
    if (action === "start") startCluster.mutate(id, options);
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
