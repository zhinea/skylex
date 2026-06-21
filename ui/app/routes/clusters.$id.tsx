import { useEffect, useMemo, useRef, useState } from "react";
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
import { Card } from "~/components/Card";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { PageSpinner } from "~/components/Spinner";
import { SettingInput, curatedSettings, validateSettingValue } from "~/components/SettingInput";
import { AgentStatus } from "~/components/AgentStatus";
import type { Node } from "~/hooks/useNodes";
import type { Cluster } from "~/hooks/useClusters";
import { LayoutDashboard, Link2, Settings as SettingsIcon, ShieldAlert } from "lucide-react";

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
    return (
      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200">
        not installed
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
        {version || "installed"}
      </span>
      <span
        className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
          dataInitialized
            ? "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200"
            : "bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400"
        }`}
      >
        {dataInitialized ? "data ready" : "not initialized"}
      </span>
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
    <Card title="Service Lifecycle">
      <div className="space-y-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div className="rounded-lg border border-border px-3 py-2">
            <div className="text-xs text-muted-foreground">Ready Nodes</div>
            <div className="text-lg font-semibold text-foreground">{readyNodes.length}/{nodes.length}</div>
          </div>
          <div className="rounded-lg border border-border px-3 py-2">
            <div className="text-xs text-muted-foreground">Running</div>
            <div className="text-lg font-semibold text-foreground">{runningNodes.length}</div>
          </div>
          <div className="rounded-lg border border-border px-3 py-2">
            <div className="text-xs text-muted-foreground">Stopped</div>
            <div className="text-lg font-semibold text-foreground">{stoppedNodes.length}</div>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button onClick={() => onAction("start")} disabled={busy || !hasReadyNodes} className="rounded-lg bg-green-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-green-700 disabled:opacity-50">
            Start
          </button>
          <button onClick={() => onAction("pause")} disabled={busy || !hasReadyNodes} className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800">
            Pause
          </button>
          <button onClick={() => onAction("restart")} disabled={busy || !hasReadyNodes} className="rounded-lg bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50">
            Restart
          </button>
          {busy && <span className="text-sm text-muted-foreground">Lifecycle command pending...</span>}
        </div>
        {disabledReason && <p className="text-sm text-muted-foreground">{disabledReason}</p>}
        {error && <p className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300">{error}</p>}
      </div>
    </Card>
  );
}

function levelColor(level: string): string {
  switch (level.toLowerCase()) {
    case "error":
      return "text-red-700 dark:text-red-400";
    case "warn":
      return "text-yellow-700 dark:text-yellow-400";
    case "info":
      return "text-blue-700 dark:text-blue-400";
    default:
      return "text-gray-500 dark:text-gray-400";
  }
}

function statusDetailColor(detail: string): string {
  switch (detail) {
    case "healthy":
      return "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400";
    case "running":
      return "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400";
    case "syncing_replica":
      return "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400";
    case "initializing_data_directory":
      return "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400";
    case "installation_conflict":
      return "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400";
    case "waiting_for_postgres":
      return "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400";
    case "stopped":
      return "bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400";
    default:
      return "bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400";
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
    <button
      onClick={handleCopy}
      title="Copy to clipboard"
      className="ml-2 inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-300 dark:hover:bg-gray-600 transition-colors"
    >
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

function ConnectionRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start gap-2 py-2 border-b border-gray-100 dark:border-gray-800 last:border-b-0">
      <dt className="w-36 shrink-0 text-xs text-gray-500 dark:text-gray-400 pt-0.5">{label}</dt>
      <dd className="flex-1 min-w-0 flex items-start gap-1">
        <code className="text-xs font-mono text-gray-900 dark:text-gray-100 break-all">{value}</code>
        <CopyButton text={value} />
      </dd>
    </div>
  );
}

function RoleStatusBadge({ status }: { status: string }) {
  const classes =
    status === "ready"
      ? "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400"
      : status === "failed"
        ? "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400"
        : status === "deleting"
          ? "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300"
          : "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400";
  return (
    <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${classes}`}>
      {status || "pending"}
    </span>
  );
}

function connectionURI(host: string, port: number, roleName: string, password?: string) {
  const user = roleName || "<user>";
  const pass = password || "<password>";
  return `postgresql://${user}:${pass}@${host}:${port}/postgres?sslmode=prefer`;
}

function databaseConnectionURI(host: string, port: number, databaseName: string, roleName?: string, password?: string) {
  const user = roleName || "<user>";
  const pass = password || "<password>";
  return `postgresql://${user}:${pass}@${host}:${port}/${databaseName}?sslmode=prefer`;
}

function databasePsqlCommand(host: string, port: number, databaseName: string, roleName?: string) {
  return `psql "host=${host} port=${port} dbname=${databaseName} user=${roleName || "<user>"} sslmode=prefer"`;
}

function ManagedRolesCard({
  clusterId,
  host,
  port,
  revealed,
  onReveal,
  onDismissReveal,
}: {
  clusterId: string;
  host: string;
  port: number;
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
    <Card title="Managed Roles">
      <div className="space-y-5">
        <p className="text-sm text-gray-500 dark:text-gray-400">
          Create application users safely. Passwords are generated by Skylex and shown only once after create or rotate.
        </p>

        {revealed && (
          <div className="rounded-lg border border-green-200 bg-green-50 px-4 py-3 dark:border-green-800 dark:bg-green-900/20">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0 flex-1 space-y-2">
                <p className="text-sm font-medium text-green-900 dark:text-green-100">
                  One-time password for {revealed.role.roleName}
                </p>
                <ConnectionRow label="Password" value={revealed.password} />
                {host && <ConnectionRow label="Connection URI" value={connectionURI(host, port, revealed.role.roleName, revealed.password)} />}
                <p className="text-xs text-green-700 dark:text-green-300">
                  Save this password now. Skylex will not show it again.
                </p>
              </div>
              <button
                onClick={onDismissReveal}
                className="shrink-0 rounded border border-green-300 px-2 py-1 text-xs font-medium text-green-800 hover:bg-green-100 dark:border-green-700 dark:text-green-200 dark:hover:bg-green-900/40"
              >
                Dismiss
              </button>
            </div>
          </div>
        )}

        <div className="rounded-lg border border-gray-200 p-4 dark:border-gray-700">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-[1fr_12rem_auto] md:items-end">
            <div>
              <label className="mb-1 block text-xs text-gray-600 dark:text-gray-400">Role Name</label>
              <input
                type="text"
                value={roleName}
                onChange={(e) => setRoleName(e.target.value)}
                placeholder="app_user"
                className="w-full rounded border border-gray-300 bg-white px-2 py-1.5 text-sm text-gray-900 placeholder-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-white"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs text-gray-600 dark:text-gray-400">Role Kind</label>
              <select
                value={roleKind}
                onChange={(e) => setRoleKind(e.target.value)}
                className="w-full rounded border border-gray-300 bg-white px-2 py-1.5 text-sm text-gray-900 dark:border-gray-600 dark:bg-gray-800 dark:text-white"
              >
                <option value="read_write">Read/Write</option>
                <option value="read_only">Read Only</option>
                <option value="admin">Admin</option>
                <option value="custom">Custom</option>
              </select>
            </div>
            <button
              onClick={handleCreate}
              disabled={!canSubmit}
              className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
            >
              {createRole.isPending ? "Creating..." : "Create Role"}
            </button>
          </div>
          {error && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{error}</p>}
        </div>

        {isLoading ? (
          <PageSpinner />
        ) : roles.length === 0 ? (
          <p className="rounded-lg border border-gray-200 py-6 text-center text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400">
            No managed roles yet.
          </p>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-900/40">
                  <th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Role</th>
                  <th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Kind</th>
                  <th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Status</th>
                  <th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Connection URI</th>
                  <th className="px-3 py-2 text-right font-medium text-gray-600 dark:text-gray-400">Actions</th>
                </tr>
              </thead>
              <tbody>
                {roles.map((role) => (
                  <tr key={role.id} className="border-b border-gray-100 last:border-b-0 dark:border-gray-800">
                    <td className="px-3 py-2 font-medium text-gray-900 dark:text-white">{role.roleName}</td>
                    <td className="px-3 py-2 text-gray-700 dark:text-gray-300">{role.roleKind.replace("_", " ")}</td>
                    <td className="px-3 py-2"><RoleStatusBadge status={role.status} /></td>
                    <td className="px-3 py-2">
                      {host ? (
                        <div className="flex items-center gap-1">
                          <code className="max-w-[28rem] truncate text-xs text-gray-700 dark:text-gray-300">
                            {connectionURI(host, port, role.roleName)}
                          </code>
                          <CopyButton text={connectionURI(host, port, role.roleName)} />
                        </div>
                      ) : (
                        <span className="text-xs text-gray-500 dark:text-gray-400">Endpoint unavailable</span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-right">
                      <div className="flex justify-end gap-3">
                        <button
                          onClick={() => handleRotate(role)}
                          disabled={role.status === "deleting" || rotatePassword.isPending}
                          className="text-xs font-medium text-blue-600 hover:underline disabled:opacity-50 dark:text-blue-400"
                        >
                          Rotate
                        </button>
                        <button
                          onClick={() => handleDelete(role)}
                          disabled={role.status === "deleting" || deleteRole.isPending}
                          className="text-xs font-medium text-red-600 hover:underline disabled:opacity-50 dark:text-red-400"
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </Card>
  );
}

function ManagedDatabasesCard({
  clusterId,
  host,
  port,
  revealedRole,
}: {
  clusterId: string;
  host: string;
  port: number;
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
    <Card title="Managed Databases">
      <div className="space-y-5">
        <p className="text-sm text-gray-500 dark:text-gray-400">
          Create application databases and optionally attach ownership to a managed read/write or admin role.
        </p>

        <div className="rounded-lg border border-gray-200 p-4 dark:border-gray-700">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-[1fr_14rem_auto] md:items-end">
            <div>
              <label className="mb-1 block text-xs text-gray-600 dark:text-gray-400">Database Name</label>
              <input
                type="text"
                value={databaseName}
                onChange={(e) => setDatabaseName(e.target.value)}
                placeholder="app_db"
                className="w-full rounded border border-gray-300 bg-white px-2 py-1.5 text-sm text-gray-900 placeholder-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-white"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs text-gray-600 dark:text-gray-400">Owner Role</label>
              <select
                value={ownerRoleId}
                onChange={(e) => setOwnerRoleId(e.target.value)}
                className="w-full rounded border border-gray-300 bg-white px-2 py-1.5 text-sm text-gray-900 dark:border-gray-600 dark:bg-gray-800 dark:text-white"
              >
                <option value="">Skylex admin default</option>
                {roles.map((role) => (
                  <option key={role.id} value={role.id}>{role.roleName}</option>
                ))}
              </select>
            </div>
            <button
              onClick={handleCreate}
              disabled={!canSubmit}
              className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
            >
              {createDatabase.isPending ? "Creating..." : "Create Database"}
            </button>
          </div>
          {error && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{error}</p>}
        </div>

        {isLoading ? (
          <PageSpinner />
        ) : databases.length === 0 ? (
          <p className="rounded-lg border border-gray-200 py-6 text-center text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400">
            No managed databases yet.
          </p>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-900/40">
                  <th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Database</th>
                  <th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Owner</th>
                  <th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Status</th>
                  <th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">URI Template</th>
                  <th className="px-3 py-2 text-right font-medium text-gray-600 dark:text-gray-400">Actions</th>
                </tr>
              </thead>
              <tbody>
                {databases.map((database) => (
                  <tr key={database.id} className="border-b border-gray-100 last:border-b-0 dark:border-gray-800">
                    <td className="px-3 py-2 font-medium text-gray-900 dark:text-white">{database.databaseName}</td>
                    <td className="px-3 py-2 text-gray-700 dark:text-gray-300">{database.ownerRoleName || "Skylex admin"}</td>
                    <td className="px-3 py-2"><RoleStatusBadge status={database.status} /></td>
                    <td className="px-3 py-2">
                      {host ? (
                        <div className="space-y-1">
                          <div className="flex items-center gap-1">
                            <code className="max-w-[28rem] truncate text-xs text-gray-700 dark:text-gray-300">
                              {databaseConnectionURI(
                                host,
                                port,
                                database.databaseName,
                                database.ownerRoleName,
                                revealedDatabasePassword(database),
                              )}
                            </code>
                            <CopyButton text={databaseConnectionURI(
                              host,
                              port,
                              database.databaseName,
                              database.ownerRoleName,
                              revealedDatabasePassword(database),
                            )} />
                          </div>
                          <div className="flex items-center gap-1">
                            <code className="max-w-[28rem] truncate text-xs text-gray-500 dark:text-gray-400">
                              {databasePsqlCommand(host, port, database.databaseName, database.ownerRoleName)}
                            </code>
                            <CopyButton text={databasePsqlCommand(host, port, database.databaseName, database.ownerRoleName)} />
                          </div>
                        </div>
                      ) : (
                        <span className="text-xs text-gray-500 dark:text-gray-400">Endpoint unavailable</span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-right">
                      <button
                        onClick={() => handleDelete(database)}
                        disabled={database.status === "deleting" || deleteDatabase.isPending}
                        className="text-xs font-medium text-red-600 hover:underline disabled:opacity-50 dark:text-red-400"
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
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
          setMessage("Access allowlists saved. Apply HBA to enforce them on nodes.");
        },
        onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to save access allowlists"),
      },
    );
  }

  function apply() {
    setMessage(null);
    applyHBA.mutate(clusterId, {
      onSuccess: () => setMessage("HBA apply queued for ready nodes."),
      onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to queue HBA apply"),
    });
  }

  if (isLoading) {
    return (
      <Card title="Network Access">
        <PageSpinner />
      </Card>
    );
  }

  const renderCIDRs = (values: string[]) =>
    values.length === 0 ? (
      <span className="text-sm text-gray-500 dark:text-gray-400">No CIDRs configured</span>
    ) : (
      <div className="flex flex-wrap gap-1">
        {values.map((cidr) => (
          <span key={cidr} className="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-700 dark:bg-gray-800 dark:text-gray-300">
            {cidr}
          </span>
        ))}
      </div>
    );

  return (
    <Card title="Network Access">
      <div className="space-y-4">
        <p className="text-sm text-gray-500 dark:text-gray-400">
          Manage Skylex-generated pg_hba.conf allowlists. Firewall and security group rules are still managed outside Skylex.
        </p>

        {editing ? (
          <div className="rounded-lg border border-blue-200 bg-blue-50 p-4 dark:border-blue-800 dark:bg-blue-900/20">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
              <label className="text-xs text-gray-600 dark:text-gray-400">
                Application CIDRs
                <textarea value={applicationCIDRs} onChange={(e) => setApplicationCIDRs(e.target.value)} rows={4} className="mt-1 w-full rounded border border-gray-300 bg-white px-2 py-1.5 font-mono text-xs text-gray-900 dark:border-gray-600 dark:bg-gray-800 dark:text-white" />
              </label>
              <label className="text-xs text-gray-600 dark:text-gray-400">
                Admin CIDRs
                <textarea value={adminCIDRs} onChange={(e) => setAdminCIDRs(e.target.value)} rows={4} className="mt-1 w-full rounded border border-gray-300 bg-white px-2 py-1.5 font-mono text-xs text-gray-900 dark:border-gray-600 dark:bg-gray-800 dark:text-white" />
              </label>
              <label className="text-xs text-gray-600 dark:text-gray-400">
                Internal Replication CIDRs
                <textarea value={replicationCIDRs} onChange={(e) => setReplicationCIDRs(e.target.value)} rows={4} className="mt-1 w-full rounded border border-gray-300 bg-white px-2 py-1.5 font-mono text-xs text-gray-900 dark:border-gray-600 dark:bg-gray-800 dark:text-white" />
              </label>
            </div>
            <p className="mt-2 text-xs text-blue-800 dark:text-blue-200">Use one CIDR per line or comma-separated values. CIDRs are validated by the API.</p>
            <div className="mt-3 flex gap-2">
              <button onClick={saveAccess} disabled={updateAccess.isPending} className="rounded-lg bg-blue-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50">
                {updateAccess.isPending ? "Saving..." : "Save Access"}
              </button>
              <button onClick={() => setEditing(false)} className="rounded-lg border border-gray-300 px-4 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800">
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            <div><div className="mb-1 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">Application CIDRs</div>{renderCIDRs(data?.allowedApplicationCidrs ?? [])}</div>
            <div><div className="mb-1 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">Admin CIDRs</div>{renderCIDRs(data?.allowedAdminCidrs ?? [])}</div>
            <div><div className="mb-1 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">Replication CIDRs</div>{renderCIDRs(data?.internalReplicationCidrs ?? [])}</div>
          </div>
        )}

        <div className="flex flex-wrap items-center gap-2">
          {!editing && <button onClick={openEditor} className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800">Edit Access</button>}
          <button onClick={apply} disabled={applyHBA.isPending} className="rounded-lg bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50">
            {applyHBA.isPending ? "Queueing..." : "Apply HBA"}
          </button>
          {message && <span className="text-sm text-gray-600 dark:text-gray-300">{message}</span>}
        </div>

        <div>
          <div className="mb-2 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">HBA Apply Status</div>
          {hbaStatuses.length === 0 ? (
            <p className="rounded-lg border border-gray-200 py-4 text-center text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400">HBA has not been applied yet.</p>
          ) : (
            <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
              <table className="w-full text-sm">
                <thead><tr className="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-900/40"><th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Node</th><th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Status</th><th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Updated</th><th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Error</th></tr></thead>
                <tbody>{hbaStatuses.map((status) => (<tr key={status.nodeId} className="border-b border-gray-100 last:border-b-0 dark:border-gray-800"><td className="px-3 py-2 text-gray-900 dark:text-white">{nodeNames.get(status.nodeId) || status.nodeId.slice(0, 8)}</td><td className="px-3 py-2"><RoleStatusBadge status={status.status} /></td><td className="px-3 py-2 text-gray-600 dark:text-gray-300">{status.updatedAt ? new Date(status.updatedAt).toLocaleString() : "-"}</td><td className="px-3 py-2 text-red-600 dark:text-red-400">{status.error || "-"}</td></tr>))}</tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </Card>
  );
}

function TLSConfigCard({ clusterId, nodes }: { clusterId: string; nodes: Node[] }) {
  const { data, isLoading } = useTLSConfig(clusterId);
  const updateTLS = useUpdateTLSConfig();
  const generateCA = useGenerateTLSCA();
  const applyTLS = useApplyTLS();
  const [editing, setEditing] = useState(false);
  const [tlsMode, setTLSMode] = useState("prefer");
  const [certFile, setCertFile] = useState("");
  const [keyFile, setKeyFile] = useState("");
  const [caFile, setCAFile] = useState("");
  const [message, setMessage] = useState<string | null>(null);

  const config = data?.config;
  const { data: caData } = useTLSCACert(clusterId, !!config?.caGenerated);
  const statuses = config?.statuses ?? [];
  const warnings = config?.warnings ?? [];
  const nodeNames = new Map(nodes.map((node) => [node.id, node.hostname]));

  function openEditor() {
    setTLSMode(config?.tlsMode ?? "prefer");
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
    generateCA.mutate(clusterId, {
      onSuccess: () => setMessage("Cluster TLS CA generated. Apply TLS to distribute CA-signed server certificates."),
      onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to generate TLS CA"),
    });
  }

  function downloadCA() {
    const pem = caData?.caCertPem;
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
      <Card title="TLS">
        <PageSpinner />
      </Card>
    );
  }

  return (
    <Card title="TLS">
      <div className="space-y-4">
        <p className="text-sm text-gray-500 dark:text-gray-400">
          Manage PostgreSQL server-side TLS settings. TLS is disabled by default. Generate a cluster CA, then Apply TLS to distribute CA-signed server certificates to ready nodes. Set both certificate and key paths only when using manually managed files.
        </p>

        {warnings.length > 0 && (
          <div className="space-y-2">
            {warnings.map((warning) => (
              <div key={warning} className="rounded-lg border border-yellow-200 bg-yellow-50 px-3 py-2 text-xs text-yellow-800 dark:border-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-200">
                {warning}
              </div>
            ))}
          </div>
        )}

        {editing ? (
          <div className="rounded-lg border border-blue-200 bg-blue-50 p-4 dark:border-blue-800 dark:bg-blue-900/20">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <label className="text-xs text-gray-600 dark:text-gray-400">
                TLS Mode
                <select value={tlsMode} onChange={(e) => setTLSMode(e.target.value)} className="mt-1 w-full rounded border border-gray-300 bg-white px-2 py-1.5 text-sm text-gray-900 dark:border-gray-600 dark:bg-gray-800 dark:text-white">
                  <option value="disabled">disabled</option>
                  <option value="prefer">prefer</option>
                  <option value="required">required</option>
                </select>
              </label>
              <label className="text-xs text-gray-600 dark:text-gray-400">
                Server Certificate Path (optional)
                <input value={certFile} onChange={(e) => setCertFile(e.target.value)} placeholder="/etc/skylex/postgres/server.crt" className="mt-1 w-full rounded border border-gray-300 bg-white px-2 py-1.5 font-mono text-xs text-gray-900 dark:border-gray-600 dark:bg-gray-800 dark:text-white" />
              </label>
              <label className="text-xs text-gray-600 dark:text-gray-400">
                Server Key Path (optional)
                <input value={keyFile} onChange={(e) => setKeyFile(e.target.value)} placeholder="/etc/skylex/postgres/server.key" className="mt-1 w-full rounded border border-gray-300 bg-white px-2 py-1.5 font-mono text-xs text-gray-900 dark:border-gray-600 dark:bg-gray-800 dark:text-white" />
              </label>
              <label className="text-xs text-gray-600 dark:text-gray-400">
                CA File Path (optional)
                <input value={caFile} onChange={(e) => setCAFile(e.target.value)} placeholder="/etc/skylex/postgres/ca.crt" className="mt-1 w-full rounded border border-gray-300 bg-white px-2 py-1.5 font-mono text-xs text-gray-900 dark:border-gray-600 dark:bg-gray-800 dark:text-white" />
              </label>
            </div>
            <p className="mt-2 text-xs text-blue-800 dark:text-blue-200">Skylex stores only file paths in manual mode. Empty certificate/key paths use Skylex-generated CA-signed certificates after you generate a cluster CA.</p>
            <div className="mt-3 flex gap-2">
              <button onClick={saveTLS} disabled={updateTLS.isPending} className="rounded-lg bg-blue-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50">
                {updateTLS.isPending ? "Saving..." : "Save TLS"}
              </button>
              <button onClick={() => setEditing(false)} className="rounded-lg border border-gray-300 px-4 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800">
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <dl className="rounded-lg border border-gray-200 px-3 dark:border-gray-700">
            <ConnectionRow label="TLS Mode" value={config?.tlsMode ?? "prefer"} />
            <ConnectionRow label="Cluster CA" value={config?.caGenerated ? "Generated" : "Not generated"} />
            <ConnectionRow label="Server Certificate" value={config?.certFile || "Skylex-managed CA-signed per node"} />
            <ConnectionRow label="Server Key" value={config?.keyFile || "Skylex-managed CA-signed per node"} />
            <ConnectionRow label="CA File" value={config?.caFile || (config?.caGenerated ? "Skylex-managed per node" : "Not configured")} />
          </dl>
        )}

        <div className="flex flex-wrap items-center gap-2">
          {!editing && <button onClick={openEditor} className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800">Edit TLS</button>}
          <button onClick={generate} disabled={generateCA.isPending} className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800">
            {generateCA.isPending ? "Generating..." : config?.caGenerated ? "Regenerate CA Cert" : "Generate CA Cert"}
          </button>
          {config?.caGenerated && <button onClick={downloadCA} disabled={!caData?.caCertPem} className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800">Download CA Cert</button>}
          <button onClick={apply} disabled={applyTLS.isPending || (config?.tlsMode !== "disabled" && !config?.caGenerated && !config?.certFile)} className="rounded-lg bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50">
            {applyTLS.isPending ? "Queueing..." : "Apply TLS"}
          </button>
          {message && <span className="text-sm text-gray-600 dark:text-gray-300">{message}</span>}
        </div>

        <div>
          <div className="mb-2 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">TLS Apply Status</div>
          {statuses.length === 0 ? (
            <p className="rounded-lg border border-gray-200 py-4 text-center text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400">TLS has not been applied yet.</p>
          ) : (
            <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
              <table className="w-full text-sm">
                <thead><tr className="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-900/40"><th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Node</th><th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Mode</th><th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Status</th><th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Active</th><th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Updated</th><th className="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Error</th></tr></thead>
                <tbody>{statuses.map((status) => (<tr key={status.nodeId} className="border-b border-gray-100 last:border-b-0 dark:border-gray-800"><td className="px-3 py-2 text-gray-900 dark:text-white">{nodeNames.get(status.nodeId) || status.nodeId.slice(0, 8)}</td><td className="px-3 py-2 text-gray-600 dark:text-gray-300">{status.requestedTlsMode}</td><td className="px-3 py-2"><RoleStatusBadge status={status.status} /></td><td className="px-3 py-2 text-gray-600 dark:text-gray-300">{status.tlsActive ? "yes" : "no"}</td><td className="px-3 py-2 text-gray-600 dark:text-gray-300">{status.updatedAt ? new Date(status.updatedAt).toLocaleString() : "-"}</td><td className="px-3 py-2 text-red-600 dark:text-red-400">{status.error || "-"}</td></tr>))}</tbody>
              </table>
            </div>
          )}
        </div>
      </div>
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
  const [editSSLMode, setEditSSLMode] = useState("prefer");
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
  const sslMode = profile?.sslMode ?? profileData?.tlsConfig?.tlsMode ?? "prefer";
  const profileWarnings = profileData?.warnings ?? profileData?.tlsConfig?.warnings ?? [];

  const isPrimaryReady = !!primaryEndpoint || !!fallbackPrimary;

  function handleEditOpen() {
    setEditMode(profile?.endpointMode ?? "direct_primary");
    setEditPublicHost(profile?.publicHost ?? "");
    setEditPublicPort(profile?.publicPort ?? 5432);
    setEditSSLMode(profile?.sslMode ?? "prefer");
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
      <Card title="PostgreSQL Connection">
        <PageSpinner />
      </Card>
    );
  }

  if (!isPrimaryReady) {
    return (
      <Card title="PostgreSQL Connection">
        <div className="flex items-start gap-3 px-4 py-3 rounded-lg bg-yellow-50 border border-yellow-200 dark:bg-yellow-900/20 dark:border-yellow-700">
          <span className="text-yellow-600 dark:text-yellow-400 mt-0.5 text-base leading-none">⏳</span>
          <div className="text-sm">
            <p className="font-medium text-yellow-900 dark:text-yellow-100">Primary not ready</p>
            <p className="mt-1 text-yellow-700 dark:text-yellow-300">
              Connection details will appear once the primary node has PostgreSQL installed and data
              initialized.
            </p>
          </div>
        </div>
      </Card>
    );
  }

  const endpoint = displayHost ? `${displayHost}:${displayPort}` : "";
  const psqlCommand = displayHost
    ? `psql "host=${displayHost} port=${displayPort} dbname=postgres user=<user> sslmode=${sslMode}"`
    : "";
  const uriTemplate = displayHost
    ? `postgresql://<user>:<password>@${displayHost}:${displayPort}/postgres?sslmode=${sslMode}`
    : "";

  const isManualMode = profile?.endpointMode === "manual_stable_endpoint";
  const hasPublicHost = !!(profile?.publicHost);

  return (
    <Card title="PostgreSQL Connection">
      <div className="space-y-4">
        {/* Edit profile button */}
        <div className="flex justify-end gap-2">
          {savedOk && (
            <span className="text-sm text-green-600 dark:text-green-400 self-center">Profile saved.</span>
          )}
          {!editing && (
            <button
              onClick={handleEditOpen}
              className="px-3 py-1.5 text-xs font-medium rounded-lg border border-gray-300 text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800 transition-colors"
            >
              Edit Profile
            </button>
          )}
        </div>

        {/* Edit form */}
        {editing && (
          <div className="rounded-lg border border-blue-200 bg-blue-50 dark:border-blue-800 dark:bg-blue-900/20 px-4 py-4 space-y-3">
            <p className="text-sm font-medium text-blue-900 dark:text-blue-100">Connection Profile</p>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div>
                <label className="block text-xs text-gray-600 dark:text-gray-400 mb-1">Endpoint Mode</label>
                <select
                  value={editMode}
                  onChange={(e) => setEditMode(e.target.value)}
                  className="w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm px-2 py-1.5 text-gray-900 dark:text-white"
                >
                  <option value="direct_primary">Direct Primary (computed from node)</option>
                  <option value="manual_stable_endpoint">Manual Stable Endpoint</option>
                </select>
              </div>

              <div>
                <label className="block text-xs text-gray-600 dark:text-gray-400 mb-1">SSL Mode</label>
                <select
                  value={editSSLMode}
                  onChange={(e) => setEditSSLMode(e.target.value)}
                  className="w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm px-2 py-1.5 text-gray-900 dark:text-white"
                >
                  <option value="disabled">disabled</option>
                  <option value="prefer">prefer</option>
                  <option value="required">required</option>
                </select>
              </div>

              <div>
                <label className="block text-xs text-gray-600 dark:text-gray-400 mb-1">Public Host</label>
                <input
                  type="text"
                  value={editPublicHost}
                  onChange={(e) => setEditPublicHost(e.target.value)}
                  placeholder="e.g. pg.example.com"
                  className="w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm px-2 py-1.5 text-gray-900 dark:text-white placeholder-gray-400"
                />
              </div>

              <div>
                <label className="block text-xs text-gray-600 dark:text-gray-400 mb-1">Public Port</label>
                <input
                  type="number"
                  min={1}
                  max={65535}
                  value={editPublicPort}
                  onChange={(e) => setEditPublicPort(Number(e.target.value))}
                  className="w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm px-2 py-1.5 text-gray-900 dark:text-white"
                />
              </div>

              <div className="md:col-span-2">
                <label className="block text-xs text-gray-600 dark:text-gray-400 mb-1">
                  Allowed CIDRs <span className="text-gray-400">(comma-separated, e.g. 10.0.0.0/8, 192.168.1.0/24)</span>
                </label>
                <input
                  type="text"
                  value={editCIDRs}
                  onChange={(e) => setEditCIDRs(e.target.value)}
                  placeholder="e.g. 10.0.0.0/8, 0.0.0.0/0"
                  className="w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm px-2 py-1.5 text-gray-900 dark:text-white placeholder-gray-400"
                />
              </div>
            </div>

            {saveError && (
              <p className="text-sm text-red-600 dark:text-red-400">{saveError}</p>
            )}

            <div className="flex gap-2 pt-1">
              <button
                onClick={handleSave}
                disabled={updateProfile.isPending}
                className="px-4 py-1.5 text-sm font-medium bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-colors"
              >
                {updateProfile.isPending ? "Saving..." : "Save"}
              </button>
              <button
                onClick={() => setEditing(false)}
                className="px-4 py-1.5 text-sm font-medium rounded-lg border border-gray-300 text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800 transition-colors"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {/* Warnings */}
        <div className="space-y-2">
          {!isManualMode && (
            <div className="flex items-start gap-2 rounded-lg border border-yellow-200 bg-yellow-50 px-3 py-2 dark:border-yellow-800 dark:bg-yellow-900/20">
              <span className="mt-0.5 shrink-0 text-yellow-600 dark:text-yellow-400">⚠</span>
              <p className="text-xs text-yellow-800 dark:text-yellow-200">
                <span className="font-medium">Direct node endpoint:</span> This endpoint points directly
                to the primary node and may change after a failover. Configure a stable endpoint (VIP, DNS, or
                proxy) via Edit Profile.
              </p>
            </div>
          )}
          {isManualMode && !hasPublicHost && (
            <div className="flex items-start gap-2 rounded-lg border border-yellow-200 bg-yellow-50 px-3 py-2 dark:border-yellow-800 dark:bg-yellow-900/20">
              <span className="mt-0.5 shrink-0 text-yellow-600 dark:text-yellow-400">⚠</span>
              <p className="text-xs text-yellow-800 dark:text-yellow-200">
                <span className="font-medium">Manual stable endpoint mode is active</span> but no public host is
                set. Set a public host via Edit Profile or switch to Direct Primary.
              </p>
            </div>
          )}
          <div className="flex items-start gap-2 rounded-lg border border-blue-200 bg-blue-50 px-3 py-2 dark:border-blue-800 dark:bg-blue-900/20">
            <span className="mt-0.5 shrink-0 text-blue-600 dark:text-blue-400">ℹ</span>
            <p className="text-xs text-blue-800 dark:text-blue-200">
              <span className="font-medium">Firewall:</span> Ensure port {displayPort} is open from your
              application to the PostgreSQL node. Skylex does not manage firewall or security group
              rules.
            </p>
          </div>
          {profileWarnings.map((warning) => (
            <div key={warning} className="flex items-start gap-2 rounded-lg border border-yellow-200 bg-yellow-50 px-3 py-2 dark:border-yellow-800 dark:bg-yellow-900/20">
              <span className="mt-0.5 shrink-0 text-yellow-600 dark:text-yellow-400">⚠</span>
              <p className="text-xs text-yellow-800 dark:text-yellow-200">{warning}</p>
            </div>
          ))}
        </div>

        {/* Connection details */}
        {endpoint && (
          <dl className="rounded-lg border border-gray-200 dark:border-gray-700 px-3 divide-y-0">
            <ConnectionRow label="Primary Endpoint" value={endpoint} />
            <ConnectionRow label="Default Database" value="postgres" />
            <ConnectionRow label="SSL Mode" value={sslMode} />
            <ConnectionRow label="Service Location" value={serviceLocation} />
            {psqlCommand && <ConnectionRow label="psql Command" value={psqlCommand} />}
            {uriTemplate && <ConnectionRow label="URI Template" value={uriTemplate} />}
          </dl>
        )}

        {endpoint && (
          <p className="text-xs text-gray-500 dark:text-gray-400">
            Replace <code className="font-mono">&lt;user&gt;</code> and{" "}
            <code className="font-mono">&lt;password&gt;</code> with your PostgreSQL credentials.
            Stored passwords are never displayed; generated passwords appear only once after create or rotate.
          </p>
        )}

        <ManagedRolesCard
          clusterId={clusterId}
          host={displayHost}
          port={displayPort}
          revealed={revealedRole}
          onReveal={setRevealedRole}
          onDismissReveal={() => setRevealedRole(null)}
        />
        <ManagedDatabasesCard clusterId={clusterId} host={displayHost} port={displayPort} revealedRole={revealedRole} />
        <TLSConfigCard clusterId={clusterId} nodes={nodes} />
        <NetworkAccessCard clusterId={clusterId} nodes={nodes} />

        {/* Replica endpoints */}
        {(replicaEndpoints.length > 0 || fallbackReplicas.length > 0) && (
          <div>
            <div className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide mb-2">
              Replica Endpoints (read-only)
            </div>
            <dl className="rounded-lg border border-gray-200 dark:border-gray-700 px-3 divide-y-0">
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
      </div>
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
    <Card title="Native PostgreSQL Conflict">
      <div className="space-y-4">
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 dark:border-red-800 dark:bg-red-900/20">
          <p className="text-sm font-medium text-red-900 dark:text-red-100">
            Existing native PostgreSQL or data was found on {nodes.length} selected node{nodes.length === 1 ? "" : "s"}.
          </p>
          <p className="mt-1 text-sm text-red-700 dark:text-red-200">
            Skylex is paused to avoid unplanned data loss. Choose Use Existing to adopt the detected installation, Remove & Reinstall to purge packages and the configured data directory, or Abort Cluster Creation.
          </p>
        </div>

        {nodes.map((node) => (
          <div key={node.id} className="rounded-lg border border-gray-200 p-4 dark:border-gray-700">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
              <div>
                <div className="font-medium text-gray-900 dark:text-white">{node.hostname}</div>
                <div className="mt-1 text-sm text-gray-600 dark:text-gray-300">
                  {node.conflictDetails || "Existing PostgreSQL installation or data directory content detected."}
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <button
                  onClick={() => onResolve(node.id, "ADOPT")}
                  disabled={pending}
                  className="rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
                >
                  Use Existing
                </button>
                <button
                  onClick={() => onResolve(node.id, "PURGE")}
                  disabled={pending}
                  className="rounded-lg bg-red-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:opacity-50"
                >
                  Remove & Reinstall
                </button>
                <button
                  onClick={() => onResolve(node.id, "ABORT")}
                  disabled={pending}
                  className="rounded-lg border border-gray-300 px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:opacity-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800"
                >
                  Abort Cluster Creation
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>
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
    <Card title="PostgreSQL Settings">
      {isLoading ? (
        <PageSpinner />
      ) : (
        <div className="space-y-4">
          <p className="text-sm text-gray-500 dark:text-gray-400">
            Changes are validated, persisted, and applied to every node in the cluster.
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
              <p key={`${s.key}-err`} className="text-sm text-red-600 dark:text-red-400">
                {s.label}: {errors[s.key]}
              </p>
            ) : null,
          )}
          <div className="flex items-center gap-3 pt-2">
            <button
              onClick={handleSave}
              disabled={!dirty || update.isPending}
              className="px-4 py-2 text-sm font-medium bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-colors"
            >
              {update.isPending ? "Saving..." : "Apply Settings"}
            </button>
            {saved && (
              <span className="text-sm text-green-600 dark:text-green-400">Settings queued for all nodes.</span>
            )}
            {update.isError && (
              <span className="text-sm text-red-600 dark:text-red-400">
                {update.error instanceof Error ? update.error.message : "Failed to update settings"}
              </span>
            )}
          </div>
        </div>
      )}
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
    <Card title="Installation Progress">
      <div className="space-y-4">
        <div>
          <div className="flex justify-between text-sm mb-1">
            <span className="text-gray-600 dark:text-gray-400">Provisioning</span>
            <span className="text-gray-900 dark:text-white font-medium">
              {readyNodes.length}/{totalNodes} nodes ready
            </span>
          </div>
          <div className="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2.5">
            <div
              className={`h-2.5 rounded-full transition-all duration-500 ${
                progressPct === 100 ? "bg-green-500" : progressPct > 0 ? "bg-blue-500" : "bg-yellow-500"
              }`}
              style={{ width: `${progressPct}%` }}
            />
          </div>
        </div>

        {nodeList.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-400">Node</th>
                  <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-400">Location</th>
                  <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-400">Install State</th>
                </tr>
              </thead>
              <tbody>
                {nodeList.map((n) => (
                  <tr key={n.id} className="border-b border-gray-100 dark:border-gray-800">
                    <td className="px-3 py-2">
                      <div className="text-gray-900 dark:text-white font-medium">{n.hostname}</div>
                      <div className="text-xs text-gray-500 dark:text-gray-400">{n.role}</div>
                    </td>
                    <td className="px-3 py-2 text-gray-700 dark:text-gray-300">
                      {n.serviceLocation === "SERVICE_LOCATION_DOCKER" ? "Dockerized" : "Native"}
                    </td>
                    <td className="px-3 py-2">
                      <PgStatusBadges
                        installed={n.postgresInstalled}
                        version={n.postgresVersion}
                        dataInitialized={n.postgresDataInitialized}
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <div>
          <div className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide mb-2">
            Recent Installation Logs
          </div>
          {tailLogs.length === 0 ? (
            <p className="text-sm text-gray-500 dark:text-gray-400">Logs appear once agents start executing installation commands.</p>
          ) : (
            <div className="max-h-48 overflow-y-auto font-mono text-xs rounded-lg bg-gray-50 dark:bg-gray-900 border border-gray-200 dark:border-gray-700">
              {tailLogs.map((log) => (
                <div key={log.id} className="grid grid-cols-[5rem_7rem_1fr] gap-2 px-3 py-1.5 border-b border-gray-100 dark:border-gray-800 last:border-b-0">
                  <span className="text-gray-500 dark:text-gray-400">{new Date(Number(log.timestampMs)).toLocaleTimeString()}</span>
                  <span className="text-gray-700 dark:text-gray-300 truncate">{log.hostname || log.nodeId?.slice(0, 8) || "-"}</span>
                  <span className={`${levelColor(log.level)} break-all`}>{log.message}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
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
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <Link to="/clusters" className="text-sm text-muted-foreground hover:text-foreground mb-1 block transition-colors">
            ← Back to Clusters
          </Link>
          <h2 className="text-2xl font-bold tracking-tight text-foreground">{cluster.name}</h2>
        </div>
        <div className="flex items-center gap-3">
          <Badge label={cluster.status} />
          <button
            onClick={() => setDeleteClusterOpen(true)}
            className="rounded-lg border border-red-300 px-3 py-1.5 text-xs font-medium text-red-700 hover:bg-red-50 dark:border-red-800 dark:text-red-300 dark:hover:bg-red-900/20"
          >
            Delete
          </button>
        </div>
      </div>

      {/* Sub-menu Tabs */}
      <div className="flex gap-6 border-b border-border mb-6">
        <button
          onClick={() => setActiveTab("overview")}
          className={`flex items-center gap-2 pb-3 text-sm font-medium border-b-2 -mb-[2px] transition-all cursor-pointer ${
            activeTab === "overview"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <LayoutDashboard className="size-4" />
          Overview
        </button>
        <button
          onClick={() => setActiveTab("connect")}
          className={`flex items-center gap-2 pb-3 text-sm font-medium border-b-2 -mb-[2px] transition-all cursor-pointer ${
            activeTab === "connect"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <Link2 className="size-4" />
          Connect
        </button>
        <button
          onClick={() => setActiveTab("settings")}
          className={`flex items-center gap-2 pb-3 text-sm font-medium border-b-2 -mb-[2px] transition-all cursor-pointer ${
            activeTab === "settings"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <SettingsIcon className="size-4" />
          Settings
        </button>
        <button
          onClick={() => setActiveTab("diagnostics")}
          className={`flex items-center gap-2 pb-3 text-sm font-medium border-b-2 -mb-[2px] transition-all cursor-pointer ${
            activeTab === "diagnostics"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <ShieldAlert className="size-4" />
          Diagnostics & Logs
        </button>
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

          <Card title={`Nodes (${nodes.length})`}>
            {nodes.length === 0 ? (
              <p className="text-sm text-muted-foreground py-4 text-center">
                No nodes registered for this cluster.
              </p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-border">
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">Hostname</th>
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">Role</th>
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">Address</th>
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">PostgreSQL</th>
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">Agent</th>
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">Version</th>
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">Last Seen</th>
                    </tr>
                  </thead>
                  <tbody>
                    {nodes.map((n) => (
                      <tr key={n.id} className="border-b border-border/40 last:border-0 hover:bg-muted/10">
                        <td className="px-4 py-3 text-foreground font-medium">{n.hostname}</td>
                        <td className="px-4 py-3"><Badge label={n.role} /></td>
                        <td className="px-4 py-3 text-foreground">{n.address}:{n.port}</td>
                        <td className="px-4 py-3">
                          <PgStatusBadges
                            installed={n.postgresInstalled}
                            version={n.postgresVersion}
                            dataInitialized={n.postgresDataInitialized}
                          />
                        </td>
                        <td className="px-4 py-3">
                          <AgentStatus connected={n.agentConnected} latencyMs={n.agentLatencyMs} />
                        </td>
                        <td className="px-4 py-3 text-foreground">{n.agentVersion || "-"}</td>
                        <td className="px-4 py-3 text-muted-foreground">
                          {n.lastSeen ? new Date(n.lastSeen).toLocaleString() : "-"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <Card title="Configuration">
              <dl className="space-y-2 text-sm">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <dt className="text-muted-foreground">Engine</dt>
                  <dd className="text-foreground font-medium">{cluster.config?.engine || "POSTGRESQL"}</dd>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <dt className="text-muted-foreground">Version</dt>
                  <dd className="text-foreground font-medium">{cluster.config?.version || "16"}</dd>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <dt className="text-muted-foreground">Service Location</dt>
                  <dd className="text-foreground font-medium">
                    {cluster.serviceLocation === "SERVICE_LOCATION_DOCKER" || cluster.config?.serviceLocation === "SERVICE_LOCATION_DOCKER"
                      ? "Dockerized"
                      : "Native"}
                  </dd>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <dt className="text-muted-foreground">Replication</dt>
                  <dd className="text-foreground font-medium">{cluster.config?.replicationMode || "ASYNC"}</dd>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <dt className="text-muted-foreground">Replicas</dt>
                  <dd className="text-foreground font-medium">{cluster.config?.replicaCount || 0}</dd>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <dt className="text-muted-foreground">PITR</dt>
                  <dd className="text-foreground font-medium">{cluster.config?.pitrEnabled ? "Enabled" : "Disabled"}</dd>
                </div>
                <div className="flex justify-between py-1">
                  <dt className="text-muted-foreground">Created</dt>
                  <dd className="text-foreground font-medium">{new Date(cluster.createdAt).toLocaleString()}</dd>
                </div>
              </dl>
            </Card>

            <Card title="Labels">
              {cluster.config?.labels && Object.keys(cluster.config.labels).length > 0 ? (
                <div className="flex flex-wrap gap-2">
                  {Object.entries(cluster.config.labels).map(([k, v]) => (
                    <span key={k} className="px-2.5 py-1 bg-secondary text-secondary-foreground border rounded-md text-xs font-mono">
                      {k}: {v}
                    </span>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">No labels configured</p>
              )}
            </Card>
          </div>
        </div>
      )}

      {activeTab === "connect" && (
        <div className="space-y-6">
          {nodes.length > 0 ? (
            <PostgreSQLConnectionCard clusterId={id || ""} nodes={nodes} cluster={cluster} />
          ) : (
            <p className="text-sm text-muted-foreground py-8 text-center bg-card border rounded-xl">
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
          <Card title="Diagnostics">
            <div className="space-y-4">
              {/* Overall progress bar */}
              <div>
                <div className="flex justify-between text-sm mb-1">
                  <span className="text-muted-foreground">Cluster Progress</span>
                  <span className="text-foreground font-medium">
                    {onlineNodes.length}/{totalNodes} nodes healthy
                  </span>
                </div>
                <div className="w-full bg-muted rounded-full h-2">
                  <div
                    className={`h-2 rounded-full transition-all duration-500 ${
                      progressPct === 100
                        ? "bg-green-500"
                        : progressPct > 50
                          ? "bg-blue-500"
                          : "bg-yellow-500"
                    }`}
                    style={{ width: `${progressPct}%` }}
                  />
                </div>
              </div>

              {/* Last error + suggested fix */}
              {lastErrorLog && (
                <div className="flex items-start gap-3 px-4 py-3 rounded-md bg-destructive/10 border border-destructive/20">
                  <span className="text-destructive font-bold mt-0.5">✗</span>
                  <div className="text-sm">
                    <p className="text-destructive font-medium">Last Error</p>
                    <p className="text-foreground/90 mt-1 font-mono text-xs">
                      [{lastErrorLog.hostname || lastErrorLog.nodeId?.slice(0, 8)}] {lastErrorLog.message}
                    </p>
                    {suggestedFix && (
                      <p className="text-muted-foreground mt-1.5">
                        <span className="font-medium text-foreground">Suggested fix:</span> {suggestedFix}
                      </p>
                    )}
                  </div>
                </div>
              )}

              {/* Per-node status with actions */}
              {nodes.length > 0 && (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-border">
                        <th className="text-left px-3 py-2 font-medium text-muted-foreground">Node</th>
                        <th className="text-left px-3 py-2 font-medium text-muted-foreground">Status</th>
                        <th className="text-left px-3 py-2 font-medium text-muted-foreground">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {nodes.map((n) => (
                        <tr key={n.id} className="border-b border-border/40 last:border-0">
                          <td className="px-3 py-2">
                            <div className="text-foreground font-medium">{n.hostname}</div>
                            <div className="text-xs text-muted-foreground">{n.role}</div>
                          </td>
                          <td className="px-3 py-2">
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
                          </td>
                          <td className="px-3 py-2">
                            <div className="flex items-center gap-3">
                              <button
                                onClick={() => { setActionNodeId(n.id); setActionType("restart"); }}
                                className="text-xs text-primary hover:underline"
                              >
                                Restart
                              </button>
                              {n.role === "NODE_ROLE_REPLICA" && (
                                <button
                                  onClick={() => { setActionNodeId(n.id); setActionType("rejoin"); }}
                                  className="text-xs text-muted-foreground hover:text-foreground hover:underline"
                                >
                                  Re-sync
                                </button>
                              )}
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </Card>

          <Card title="Command Logs">
            {logs.length === 0 ? (
              <p className="text-sm text-muted-foreground py-4 text-center">
                No command logs yet. Logs appear while the agent executes commands.
              </p>
            ) : (
              <div className="overflow-x-auto max-h-[500px] overflow-y-auto font-mono text-xs border rounded-md">
                <table className="w-full">
                  <tbody>
                    {logs.map((log) => (
                      <tr key={log.id} className="border-b border-border/40 last:border-0 hover:bg-muted/5">
                        <td className="px-3 py-2 whitespace-nowrap text-muted-foreground">
                          {new Date(Number(log.timestampMs)).toLocaleTimeString()}
                        </td>
                        <td className="px-3 py-2 whitespace-nowrap text-foreground/80">
                          {log.hostname || log.nodeId?.slice(0, 8) || "-"}
                        </td>
                        <td className="px-3 py-2 whitespace-nowrap">
                          <span className={levelColor(log.level)}>{log.level.toUpperCase()}</span>
                        </td>
                        <td className="px-3 py-2 text-foreground break-all">
                          {log.message}
                        </td>
                      </tr>
                    ))}
                    <tr><td colSpan={4}><div ref={logsEndRef} /></td></tr>
                  </tbody>
                </table>
              </div>
            )}
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
