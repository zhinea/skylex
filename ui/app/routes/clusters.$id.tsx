import { useEffect, useMemo, useRef, useState } from "react";
import { useParams, Link } from "react-router";
import { useCluster } from "~/hooks/useClusters";
import { useNodes } from "~/hooks/useNodes";
import { useCommandLogs, type CommandLog } from "~/hooks/useCommandLogs";
import { useClusterSettings, useUpdateClusterSettings } from "~/hooks/useClusterSettings";
import { useRestartNode } from "~/hooks/useClusters";
import { useRejoinNode } from "~/hooks/useNodes";
import { Badge } from "~/components/Badge";
import { Card } from "~/components/Card";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { PageSpinner } from "~/components/Spinner";
import { SettingInput, curatedSettings, validateSettingValue } from "~/components/SettingInput";

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
    case "waiting_for_postgres":
      return "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400";
    case "stopped":
      return "bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400";
    default:
      return "bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400";
  }
}

function SettingsCard({ clusterId }: { clusterId: string }) {
  const { data, isLoading } = useClusterSettings(clusterId);
  const update = useUpdateClusterSettings();
  const [values, setValues] = useState<Record<string, string>>({});
  const [errors, setErrors] = useState<Record<string, string | null>>({});
  const [saved, setSaved] = useState(false);

  const parameters = data?.settings?.parameters ?? {};

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

export default function ClusterDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: clusterData, isLoading: clusterLoading } = useCluster(id || "");
  const { data: nodesData } = useNodes(id || "");
  const { data: logsData } = useCommandLogs(id || "");
  const restartNode = useRestartNode();
  const rejoinNode = useRejoinNode();
  const logsEndRef = useRef<HTMLDivElement>(null);

  const [actionNodeId, setActionNodeId] = useState<string | null>(null);
  const [actionType, setActionType] = useState<"restart" | "rejoin" | null>(null);

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
  const missingPgNodes = nodes.filter((n) => !n.postgresInstalled);
  const onlineNodes = nodes.filter((n) => n.postgresInstalled && n.postgresDataInitialized);
  const totalNodes = nodes.length;

  // Compute overall progress
  const progressPct = totalNodes > 0 ? Math.round((onlineNodes.length / totalNodes) * 100) : 0;

  // Find latest error log
  const lastErrorLog = useMemo(
    () => logs.filter((l) => l.level === "error").slice(-1)[0] ?? null,
    [logs],
  );

  // Suggest fix from error message
  const suggestedFix = useMemo(() => {
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
  }, [lastErrorLog]);

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <Link to="/clusters" className="text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 mb-1 block">
            ← Back to Clusters
          </Link>
          <h2 className="text-2xl font-bold text-gray-900 dark:text-white">{cluster.name}</h2>
        </div>
        <Badge label={cluster.status} />
      </div>

      {missingPgNodes.length > 0 && (
        <div className="mb-4 flex items-start gap-3 px-4 py-3 rounded-lg bg-yellow-50 border border-yellow-200 dark:bg-yellow-900/20 dark:border-yellow-700">
          <span className="text-yellow-600 dark:text-yellow-400 mt-0.5">⚠</span>
          <p className="text-sm text-yellow-800 dark:text-yellow-200">
            {missingPgNodes.length === 1
              ? `Node "${missingPgNodes[0].hostname}" does not have PostgreSQL installed.`
              : `${missingPgNodes.length} nodes in this cluster do not have PostgreSQL installed.`}{" "}
            Install PostgreSQL on those hosts before promoting or replicating.
          </p>
        </div>
      )}

      {/* Phase 4: Diagnostics Card */}
      <div className="mb-6">
        <Card title="Diagnostics">
          <div className="space-y-4">
            {/* Overall progress bar */}
            <div>
              <div className="flex justify-between text-sm mb-1">
                <span className="text-gray-600 dark:text-gray-400">Cluster Progress</span>
                <span className="text-gray-900 dark:text-white font-medium">
                  {onlineNodes.length}/{totalNodes} nodes healthy
                </span>
              </div>
              <div className="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2.5">
                <div
                  className={`h-2.5 rounded-full transition-all duration-500 ${
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
              <div className="flex items-start gap-3 px-4 py-3 rounded-lg bg-red-50 border border-red-200 dark:bg-red-900/20 dark:border-red-700">
                <span className="text-red-600 dark:text-red-400 mt-0.5">✗</span>
                <div className="text-sm">
                  <p className="text-red-800 dark:text-red-200 font-medium">Last Error</p>
                  <p className="text-red-700 dark:text-red-300 mt-1 font-mono text-xs">
                    [{lastErrorLog.hostname || lastErrorLog.nodeId?.slice(0, 8)}] {lastErrorLog.message}
                  </p>
                  {suggestedFix && (
                    <p className="text-red-600 dark:text-red-400 mt-1">
                      <span className="font-medium">Suggested fix:</span> {suggestedFix}
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
                    <tr className="border-b border-gray-200 dark:border-gray-700">
                      <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-400">Node</th>
                      <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-400">Status</th>
                      <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-400">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {nodes.map((n) => (
                      <tr key={n.id} className="border-b border-gray-100 dark:border-gray-800">
                        <td className="px-3 py-2">
                          <div className="text-gray-900 dark:text-white font-medium">{n.hostname}</div>
                          <div className="text-xs text-gray-500 dark:text-gray-400">{n.role}</div>
                        </td>
                        <td className="px-3 py-2">
                          {n.statusDetail ? (
                            <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${statusDetailColor(n.statusDetail)}`}>
                              {n.statusDetail.replace(/_/g, " ")}
                            </span>
                          ) : (
                            <Badge label={n.role} />
                          )}
                        </td>
                        <td className="px-3 py-2">
                          <div className="flex items-center gap-2">
                            <button
                              onClick={() => { setActionNodeId(n.id); setActionType("restart"); }}
                              className="text-xs text-blue-600 hover:text-blue-800 dark:text-blue-400"
                            >
                              Restart
                            </button>
                            {n.role === "NODE_ROLE_REPLICA" && (
                              <button
                                onClick={() => { setActionNodeId(n.id); setActionType("rejoin"); }}
                                className="text-xs text-purple-600 hover:text-purple-800 dark:text-purple-400"
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
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        <Card title="Configuration">
          <dl className="space-y-2 text-sm">
            <div className="flex justify-between">
              <dt className="text-gray-500 dark:text-gray-400">Engine</dt>
              <dd className="text-gray-900 dark:text-white">{cluster.config?.engine || "POSTGRESQL"}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500 dark:text-gray-400">Version</dt>
              <dd className="text-gray-900 dark:text-white">{cluster.config?.version || "16"}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500 dark:text-gray-400">Replication</dt>
              <dd className="text-gray-900 dark:text-white">{cluster.config?.replicationMode || "ASYNC"}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500 dark:text-gray-400">Replicas</dt>
              <dd className="text-gray-900 dark:text-white">{cluster.config?.replicaCount || 0}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500 dark:text-gray-400">PITR</dt>
              <dd className="text-gray-900 dark:text-white">{cluster.config?.pitrEnabled ? "Enabled" : "Disabled"}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500 dark:text-gray-400">Created</dt>
              <dd className="text-gray-900 dark:text-white">{new Date(cluster.createdAt).toLocaleString()}</dd>
            </div>
          </dl>
        </Card>

        <Card title="Labels">
          {cluster.config?.labels && Object.keys(cluster.config.labels).length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {Object.entries(cluster.config.labels).map(([k, v]) => (
                <span key={k} className="px-2 py-1 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded text-xs">
                  {k}: {v}
                </span>
              ))}
            </div>
          ) : (
            <p className="text-sm text-gray-500 dark:text-gray-400">No labels configured</p>
          )}
        </Card>
      </div>

      <div className="mb-6">
        <SettingsCard clusterId={id || ""} />
      </div>

      <Card title={`Nodes (${nodes.length})`}>
        {nodes.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400 py-4 text-center">
            No nodes registered for this cluster.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Hostname</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Role</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Address</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">PostgreSQL</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Version</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Last Seen</th>
                </tr>
              </thead>
              <tbody>
                {nodes.map((n) => (
                  <tr key={n.id} className="border-b border-gray-100 dark:border-gray-800">
                    <td className="px-4 py-3 text-gray-900 dark:text-white font-medium">{n.hostname}</td>
                    <td className="px-4 py-3"><Badge label={n.role} /></td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{n.address}:{n.port}</td>
                    <td className="px-4 py-3">
                      <PgStatusBadges
                        installed={n.postgresInstalled}
                        version={n.postgresVersion}
                        dataInitialized={n.postgresDataInitialized}
                      />
                    </td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{n.agentVersion || "-"}</td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400">
                      {n.lastSeen ? new Date(n.lastSeen).toLocaleString() : "-"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <div className="mt-6">
        <Card title="Command Logs">
          {logs.length === 0 ? (
            <p className="text-sm text-gray-500 dark:text-gray-400 py-4 text-center">
              No command logs yet. Logs appear while the agent executes commands.
            </p>
          ) : (
            <div className="overflow-x-auto max-h-96 overflow-y-auto font-mono text-xs">
              <table className="w-full">
                <tbody>
                  {logs.map((log) => (
                    <tr key={log.id} className="border-b border-gray-100 dark:border-gray-800">
                      <td className="px-2 py-1.5 whitespace-nowrap text-gray-500 dark:text-gray-400">
                        {new Date(log.timestampMs).toLocaleTimeString()}
                      </td>
                      <td className="px-2 py-1.5 whitespace-nowrap text-gray-700 dark:text-gray-300">
                        {log.hostname || log.nodeId?.slice(0, 8) || "-"}
                      </td>
                      <td className="px-2 py-1.5 whitespace-nowrap">
                        <span className={levelColor(log.level)}>{log.level.toUpperCase()}</span>
                      </td>
                      <td className="px-2 py-1.5 text-gray-900 dark:text-white break-all">
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

      {/* Action confirmation dialogs */}
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
    </div>
  );
}