import { useState } from "react";
import { useNodes, useNodeMetrics, useDrainNode, useRejoinNode, useDeleteNode, type Node, type NodeMetric } from "~/hooks/useNodes";
import { Badge } from "~/components/Badge";
import { Card } from "~/components/Card";
import { PageSpinner } from "~/components/Spinner";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { InstallAgentModal } from "~/components/InstallAgentModal";
import { AgentStatus } from "~/components/AgentStatus";

export default function NodesPage() {
  const [page, setPage] = useState(1);
  const [installOpen, setInstallOpen] = useState(false);
  const { data, isLoading } = useNodes(undefined, page);
  const drainNode = useDrainNode();
  const rejoinNode = useRejoinNode();
  const deleteNode = useDeleteNode();
  const [drainId, setDrainId] = useState<string | null>(null);
  const [rejoinId, setRejoinId] = useState<string | null>(null);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [reconnectNode, setReconnectNode] = useState<{ id: string; hostname: string } | null>(null);
  const [detailNodeId, setDetailNodeId] = useState<string | null>(null);

  if (isLoading) return <PageSpinner />;

  const nodes = data?.nodes || [];
  const detailNode = detailNodeId ? nodes.find((n) => n.id === detailNodeId) || null : null;
  const total = data?.pagination?.total || 0;
  const pageSize = data?.pagination?.pageSize || 50;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-white">Nodes</h2>
        <button
          onClick={() => setInstallOpen(true)}
          className="px-4 py-2 text-sm font-medium bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
        >
          Add Node
        </button>
      </div>

      <Card>
        {nodes.length === 0 ? (
          <div className="py-10 text-center">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-2">
              No agents yet
            </h3>
            <p className="text-sm text-gray-500 dark:text-gray-400 mb-6 max-w-md mx-auto">
              Add your first database server by installing the Skylex agent. You&apos;ll get a copy-paste command to run on the target host.
            </p>
            <button
              onClick={() => setInstallOpen(true)}
              className="px-4 py-2 text-sm font-medium bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
            >
              Install Agent
            </button>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Hostname</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Cluster</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Role</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Address</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Agent</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Agent Version</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Last Seen</th>
                  <th className="text-right px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Actions</th>
                </tr>
              </thead>
              <tbody>
                {nodes.map((n) => (
                  <tr key={n.id} className="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-750">
                    <td className="px-4 py-3 text-gray-900 dark:text-white font-medium">{n.hostname}</td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">
                      {n.clusterId ? <span className="text-xs text-gray-500">{n.clusterId.substring(0, 8)}...</span> : "-"}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge label={n.role} />
                        {n.status === "drained" && <Badge label="drained" />}
                        {n.status === "deleting" && <Badge label="deleting" />}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{n.address}:{n.port}</td>
                    <td className="px-4 py-3">
                      <AgentStatus connected={n.agentConnected} latencyMs={n.agentLatencyMs} />
                    </td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 text-xs">{n.agentVersion || "-"}</td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 text-xs">
                      {n.lastSeen ? new Date(n.lastSeen).toLocaleString() : "-"}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-2">
                        <button
                          onClick={() => setDetailNodeId(n.id)}
                          className="text-xs text-gray-600 hover:text-gray-900 dark:text-gray-300 dark:hover:text-white"
                          title="Show node details"
                        >
                          Details
                        </button>
                        {!n.agentConnected && n.status !== "deleting" && (
                          <button
                            onClick={() => setReconnectNode({ id: n.id, hostname: n.hostname })}
                            className="text-xs text-blue-600 hover:text-blue-800 dark:text-blue-400"
                            title="Show reconnect command"
                          >
                            Reconnect
                          </button>
                        )}
                        {n.clusterId && n.status === "drained" && (
                          <button
                            onClick={() => setRejoinId(n.id)}
                            className="text-xs text-purple-600 hover:text-purple-800 dark:text-purple-400"
                            title="Rejoin cluster"
                          >
                            Rejoin
                          </button>
                        )}
                        {n.status !== "drained" && n.status !== "deleting" && (
                          <button
                            onClick={() => setDrainId(n.id)}
                            className="text-xs text-red-600 hover:text-red-800 dark:text-red-400"
                          >
                            Drain
                          </button>
                        )}
                        {!n.clusterId && n.status !== "deleting" && (
                          <button
                            onClick={() => setDeleteId(n.id)}
                            className="text-xs text-red-600 hover:text-red-800 dark:text-red-400"
                          >
                            Delete
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
        {total > pageSize && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-gray-200 dark:border-gray-700">
            <span className="text-sm text-gray-500 dark:text-gray-400">
              Page {page} of {Math.ceil(total / pageSize)}
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page === 1}
                className="px-3 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded disabled:opacity-50 text-gray-700 dark:text-gray-300"
              >
                Prev
              </button>
              <button
                onClick={() => setPage((p) => p + 1)}
                disabled={page >= Math.ceil(total / pageSize)}
                className="px-3 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded disabled:opacity-50 text-gray-700 dark:text-gray-300"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </Card>

      <ConfirmDialog
        open={!!drainId}
        title="Drain Node"
        message="This will mark the node offline and stop PostgreSQL. Are you sure?"
        confirmLabel="Drain"
        onConfirm={() => { if (drainId) { drainNode.mutate(drainId); setDrainId(null); }}}
        onCancel={() => setDrainId(null)}
      />

      <ConfirmDialog
        open={!!rejoinId}
        title="Rejoin Node"
        message="This will repoint the node to follow the current primary and restart. Any divergent data will be overwritten. Are you sure?"
        confirmLabel="Rejoin"
        onConfirm={() => { if (rejoinId) { rejoinNode.mutate(rejoinId); setRejoinId(null); }}}
        onCancel={() => setRejoinId(null)}
      />

      <ConfirmDialog
        open={!!deleteId}
        title="Delete Node"
        message="This will deactivate the agent and remove the node from Skylex. Reinstall the agent to add this machine again. Are you sure?"
        confirmLabel="Delete"
        onConfirm={() => { if (deleteId) { deleteNode.mutate(deleteId); setDeleteId(null); }}}
        onCancel={() => setDeleteId(null)}
      />

      <InstallAgentModal open={installOpen} onClose={() => setInstallOpen(false)} />
      <InstallAgentModal
        open={!!reconnectNode}
        mode="reconnect"
        hostname={reconnectNode?.hostname}
        onClose={() => setReconnectNode(null)}
      />
      <NodeDetailModal node={detailNode} onClose={() => setDetailNodeId(null)} />
    </div>
  );
}

function NodeDetailModal({ node, onClose }: { node: Node | null; onClose: () => void }) {
  const { data: metricsData } = useNodeMetrics(node?.id, 120);
  if (!node) return null;

  const latest = node.latestMetric || node;
  const history = metricsData?.metrics || [];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl max-w-4xl w-full mx-4 max-h-[90vh] overflow-y-auto" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-200 dark:border-gray-700">
          <div>
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white">{node.hostname}</h3>
            <p className="text-xs text-gray-500 dark:text-gray-400">{node.id}</p>
          </div>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div className="px-6 py-4 space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <MetricCard label="CPU" value={formatPercent(latest.cpuUsagePercent)} detail={`${latest.cpuCores || "-"} cores`} />
            <MetricCard label="Memory" value={formatPercent(latest.memoryUsagePercent)} detail={`${formatBytes(latest.memoryUsedBytes)} / ${formatBytes(latest.memoryTotalBytes)}`} />
            <MetricCard label="Disk" value={formatPercent(latest.diskUsagePercent)} detail={`${formatBytes(latest.diskUsedBytes)} / ${formatBytes(latest.diskTotalBytes)}`} />
          </div>

          <DetailSection title="Metrics History">
            <MetricSparkline label="CPU" metrics={history} valueKey="cpuUsagePercent" />
            <MetricSparkline label="Memory" metrics={history} valueKey="memoryUsagePercent" />
            <MetricSparkline label="Disk" metrics={history} valueKey="diskUsagePercent" />
          </DetailSection>

          <DetailSection title="Server">
            <DetailItem label="OS" value={latest.os || "-"} />
            <DetailItem label="Platform" value={formatPlatform(latest)} />
            <DetailItem label="Kernel" value={latest.kernelVersion || "-"} />
            <DetailItem label="Architecture" value={latest.architecture || "-"} />
            <DetailItem label="Uptime" value={formatDuration(latest.uptimeSeconds)} />
            <DetailItem label="Load average" value={formatLoad(latest)} />
          </DetailSection>

          <DetailSection title="Agent & Database">
            <DetailItem label="Agent" value={node.agentConnected ? "Connected" : "Disconnected"} />
            <DetailItem label="Agent latency" value={node.agentConnected ? `${node.agentLatencyMs} ms` : "-"} />
            <DetailItem label="Agent version" value={node.agentVersion || "-"} />
            <DetailItem label="Last seen" value={node.lastSeen ? new Date(node.lastSeen).toLocaleString() : "-"} />
            <DetailItem label="PostgreSQL" value={node.postgresInstalled ? node.postgresVersion || "Installed" : "Not installed"} />
            <DetailItem label="Data initialized" value={node.postgresDataInitialized ? "Yes" : "No"} />
            <DetailItem label="Service location" value={node.serviceLocation || "-"} />
            <DetailItem label="Docker available" value={node.dockerAvailable ? "Yes" : "No"} />
          </DetailSection>

          <DetailSection title="Node">
            <DetailItem label="Status" value={node.status} />
            <DetailItem label="Status detail" value={node.statusDetail || "-"} />
            <DetailItem label="Role" value={node.role || "-"} />
            <DetailItem label="Address" value={`${node.address}:${node.port}`} />
            <DetailItem label="Cluster" value={node.clusterId || "-"} />
            <DetailItem label="Installation state" value={node.installationState || "-"} />
          </DetailSection>
        </div>
      </div>
    </div>
  );
}

function DetailSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section>
      <h4 className="text-sm font-semibold text-gray-900 dark:text-white mb-3">{title}</h4>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">{children}</div>
    </section>
  );
}

function DetailItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 p-3">
      <div className="text-xs text-gray-500 dark:text-gray-400">{label}</div>
      <div className="mt-1 text-sm font-medium text-gray-900 dark:text-white break-all">{value}</div>
    </div>
  );
}

function MetricCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <div className="rounded-lg bg-gray-50 dark:bg-gray-900/40 border border-gray-200 dark:border-gray-700 p-4">
      <div className="text-xs text-gray-500 dark:text-gray-400">{label}</div>
      <div className="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">{value}</div>
      <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">{detail}</div>
    </div>
  );
}

function MetricSparkline({ label, metrics, valueKey }: { label: string; metrics: NodeMetric[]; valueKey: keyof Pick<NodeMetric, "cpuUsagePercent" | "memoryUsagePercent" | "diskUsagePercent"> }) {
  const points = metrics.map((m) => Number(m[valueKey])).filter((value) => Number.isFinite(value));
  const path = sparklinePath(points);
  const latest = points.length ? points[points.length - 1] : 0;

  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 p-3">
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs text-gray-500 dark:text-gray-400">{label}</span>
        <span className="text-xs font-medium text-gray-900 dark:text-white">{points.length ? formatPercent(latest) : "No history"}</span>
      </div>
      <svg viewBox="0 0 120 36" className="h-10 w-full text-blue-600 dark:text-blue-400" preserveAspectRatio="none">
        {path ? <path d={path} fill="none" stroke="currentColor" strokeWidth="2" /> : <line x1="0" y1="18" x2="120" y2="18" stroke="currentColor" strokeWidth="1" opacity="0.25" />}
      </svg>
      <div className="mt-1 text-[11px] text-gray-500 dark:text-gray-400">Last {points.length} samples</div>
    </div>
  );
}

function sparklinePath(values: number[]) {
  if (values.length < 2) return "";
  const max = Math.max(100, ...values);
  return values
    .map((value, index) => {
      const x = (index / (values.length - 1)) * 120;
      const y = 36 - (Math.max(0, Math.min(value, max)) / max) * 34 - 1;
      return `${index === 0 ? "M" : "L"}${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(" ");
}

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "-";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function formatPercent(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "-";
  return `${value.toFixed(1)}%`;
}

function formatDuration(seconds: number) {
  if (!Number.isFinite(seconds) || seconds <= 0) return "-";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

function formatLoad(node: Pick<Node, "loadAverage1M" | "loadAverage5M" | "loadAverage15M">) {
  const values = [node.loadAverage1M, node.loadAverage5M, node.loadAverage15M];
  if (values.every((value) => !value)) return "-";
  return values.map((value) => (value / 100).toFixed(2)).join(" / ");
}

function formatPlatform(node: Pick<Node, "platform" | "platformVersion">) {
  return [node.platform, node.platformVersion].filter(Boolean).join(" ") || "-";
}
