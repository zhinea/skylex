import { useState } from "react";
import { useNodes, useNodeMetrics, useDrainNode, useRejoinNode, useDeleteNode, type Node, type NodeMetric } from "~/hooks/useNodes";
import { Badge } from "~/components/Badge";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import { PageSpinner } from "~/components/Spinner";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { InstallAgentModal } from "~/components/InstallAgentModal";
import { AgentStatus } from "~/components/AgentStatus";
import { Button } from "~/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "~/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "~/components/ui/dialog";
import { PlusIcon, Network, RefreshCw, Trash2, Cpu, Database, Server } from "lucide-react";

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
  const [blockedDeleteNode, setBlockedDeleteNode] = useState<Node | null>(null);
  const [reconnectNode, setReconnectNode] = useState<{ id: string; hostname: string } | null>(null);
  const [detailNodeId, setDetailNodeId] = useState<string | null>(null);

  if (isLoading) return <PageSpinner />;

  const nodes = data?.nodes || [];
  const detailNode = detailNodeId ? nodes.find((n) => n.id === detailNodeId) || null : null;
  const total = data?.pagination?.total || 0;
  const pageSize = data?.pagination?.pageSize || 50;

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-foreground">Nodes</h2>
          <p className="text-xs text-muted-foreground mt-1">Manage database agent hosts, hardware metrics, and lifecycle states.</p>
        </div>
        <Button onClick={() => setInstallOpen(true)} variant="default" size="sm">
          <PlusIcon className="size-3.5 mr-1.5" />
          Add Node
        </Button>
      </div>

      <Card className="shadow-xs">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
            <Network className="size-4 text-muted-foreground" />
            Registered Nodes ({total})
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {nodes.length === 0 ? (
            <div className="py-16 text-center">
              <h3 className="text-sm font-semibold text-foreground mb-2">No agents yet</h3>
              <p className="text-xs text-muted-foreground mb-6 max-w-sm mx-auto">
                Add your first database server by installing the Skylex agent. You&apos;ll get a copy-paste command to run on the target host.
              </p>
              <Button onClick={() => setInstallOpen(true)} size="sm">
                Install Agent
              </Button>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/30">
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Hostname</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Cluster</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Role</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Address</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Agent</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Agent Version</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Last Seen</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {nodes.map((n) => (
                    <TableRow key={n.id} className="hover:bg-muted/30 transition-colors">
                      <TableCell className="px-6 py-3.5 text-foreground font-semibold">{n.hostname}</TableCell>
                      <TableCell className="px-6 py-3.5 text-xs text-muted-foreground font-mono">
                        {n.clusterId ? `${n.clusterId.substring(0, 8)}...` : "-"}
                      </TableCell>
                      <TableCell className="px-6 py-3.5">
                        <div className="flex flex-wrap items-center gap-1.5">
                          <Badge label={n.role} />
                          {n.status === "drained" && <Badge label="drained" />}
                          {n.status === "deleting" && <Badge label="deleting" />}
                        </div>
                      </TableCell>
                      <TableCell className="px-6 py-3.5 text-foreground/90 font-medium text-xs font-mono">{n.address}:{n.port}</TableCell>
                      <TableCell className="px-6 py-3.5">
                        <AgentStatus connected={n.agentConnected} latencyMs={n.agentLatencyMs} />
                      </TableCell>
                      <TableCell className="px-6 py-3.5 text-muted-foreground text-xs font-mono">{n.agentVersion || "-"}</TableCell>
                      <TableCell className="px-6 py-3.5 text-muted-foreground text-xs">
                        {n.lastSeen ? new Date(n.lastSeen).toLocaleString() : "-"}
                      </TableCell>
                      <TableCell className="px-6 py-3.5 text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setDetailNodeId(n.id)}
                            className="text-foreground text-xs font-medium h-7 px-2"
                          >
                            Details
                          </Button>
                          {!n.agentConnected && n.status !== "deleting" && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => setReconnectNode({ id: n.id, hostname: n.hostname })}
                              className="text-primary text-xs font-medium h-7 px-2"
                              title="Show reconnect command"
                            >
                              Reconnect
                            </Button>
                          )}
                          {n.clusterId && n.status === "drained" && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => setRejoinId(n.id)}
                              className="text-purple-600 hover:text-purple-700 hover:bg-purple-50 dark:hover:bg-purple-950/20 text-xs font-medium h-7 px-2"
                              title="Rejoin cluster"
                            >
                              Rejoin
                            </Button>
                          )}
                          {n.status !== "drained" && n.status !== "deleting" && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => setDrainId(n.id)}
                              className="text-destructive hover:bg-destructive/10 text-xs font-medium h-7 px-2"
                            >
                              Drain
                            </Button>
                          )}
                          {n.status !== "deleting" && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => {
                                if (n.clusterId) {
                                  setBlockedDeleteNode(n);
                                  return;
                                }
                                setDeleteId(n.id);
                              }}
                              className="text-destructive hover:bg-destructive/10 text-xs font-medium h-7 px-2"
                            >
                              Delete
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
          {total > pageSize && (
            <div className="flex items-center justify-between px-6 py-4 border-t border-border bg-muted/20">
              <span className="text-xs text-muted-foreground">
                Page {page} of {Math.ceil(total / pageSize)}
              </span>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={page === 1}
                  className="h-8 text-xs"
                >
                  Prev
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((p) => p + 1)}
                  disabled={page >= Math.ceil(total / pageSize)}
                  className="h-8 text-xs"
                >
                  Next
                </Button>
              </div>
            </div>
          )}
        </CardContent>
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
        onCancel={() => rejoinId && setRejoinId(null)}
      />

      <ConfirmDialog
        open={!!deleteId}
        title="Delete Node"
        message="This will deactivate the agent and remove the node from Skylex. Reinstall the agent to add this machine again. Are you sure?"
        confirmLabel="Delete"
        onConfirm={() => { if (deleteId) { deleteNode.mutate(deleteId); setDeleteId(null); }}}
        onCancel={() => setDeleteId(null)}
      />

      <BlockedDeleteDialog node={blockedDeleteNode} onClose={() => setBlockedDeleteNode(null)} />

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

function BlockedDeleteDialog({ node, onClose }: { node: Node | null; onClose: () => void }) {
  return (
    <Dialog open={!!node} onOpenChange={(isOpen) => { if (!isOpen) onClose(); }}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="text-base font-semibold text-foreground">Cannot Delete Node</DialogTitle>
          <DialogDescription className="text-xs text-muted-foreground mt-1">
            Node is currently associated with an active cluster.
          </DialogDescription>
        </DialogHeader>
        {node && (
          <div className="space-y-3 text-sm text-foreground/80 pt-2 leading-relaxed">
            <p>
              <span className="font-semibold text-foreground">{node.hostname}</span> is still assigned to a cluster, so Skylex cannot delete it directly.
            </p>
            <p>
              Direct deletion could leave the cluster without its expected primary/replica member, break replication or failover decisions, and leave cluster metadata pointing at a missing node.
            </p>
            <p className="text-xs text-muted-foreground">
              Drain the node first if you need to stop PostgreSQL, or delete/reconfigure the cluster before removing this node from Skylex.
            </p>
          </div>
        )}
        <div className="mt-4 flex justify-end">
          <Button variant="outline" size="sm" onClick={onClose}>
            Close
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function NodeDetailModal({ node, onClose }: { node: Node | null; onClose: () => void }) {
  const { data: metricsData } = useNodeMetrics(node?.id, 120);

  if (!node) return null;

  const latest = node.latestMetric || node;
  const history = metricsData?.metrics || [];

  return (
    <Dialog open={!!node} onOpenChange={(isOpen) => { if (!isOpen) onClose(); }}>
      <DialogContent className="max-w-3xl max-h-[90vh] overflow-y-auto p-6">
        <DialogHeader className="border-b border-border pb-4">
          <DialogTitle className="text-lg font-bold tracking-tight text-foreground">{node.hostname}</DialogTitle>
          <DialogDescription className="text-xs text-muted-foreground font-mono mt-1">{node.id}</DialogDescription>
        </DialogHeader>
        
        <div className="space-y-6 pt-4">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <MetricCard label="CPU" value={formatPercent(latest.cpuUsagePercent)} detail={`${latest.cpuCores || "-"} cores`} />
            <MetricCard label="Memory" value={formatPercent(latest.memoryUsagePercent)} detail={`${formatBytes(latest.memoryUsedBytes)} / ${formatBytes(latest.memoryTotalBytes)}`} />
            <MetricCard label="Disk" value={formatPercent(latest.diskUsagePercent)} detail={`${formatBytes(latest.diskUsedBytes)} / ${formatBytes(latest.diskTotalBytes)}`} />
          </div>

          <DetailSection title="Metrics History" icon={<Cpu className="size-4 text-muted-foreground" />}>
            <MetricSparkline label="CPU Usage" metrics={history} valueKey="cpuUsagePercent" />
            <MetricSparkline label="Memory Usage" metrics={history} valueKey="memoryUsagePercent" />
            <MetricSparkline label="Disk Usage" metrics={history} valueKey="diskUsagePercent" />
          </DetailSection>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <DetailSection title="System Information" icon={<Server className="size-4 text-muted-foreground" />}>
              <DetailItem label="OS" value={latest.os || "-"} />
              <DetailItem label="Platform" value={formatPlatform(latest)} />
              <DetailItem label="Kernel" value={latest.kernelVersion || "-"} />
              <DetailItem label="Architecture" value={latest.architecture || "-"} />
              <DetailItem label="Uptime" value={formatDuration(latest.uptimeSeconds)} />
              <DetailItem label="Load Average" value={formatLoad(latest)} />
            </DetailSection>

            <DetailSection title="Agent & Database" icon={<Database className="size-4 text-muted-foreground" />}>
              <DetailItem label="Agent Connection" value={node.agentConnected ? "Connected" : "Disconnected"} />
              <DetailItem label="Agent Latency" value={node.agentConnected ? `${node.agentLatencyMs} ms` : "-"} />
              <DetailItem label="Agent Version" value={node.agentVersion || "-"} />
              <DetailItem label="Last Seen" value={node.lastSeen ? new Date(node.lastSeen).toLocaleString() : "-"} />
              <DetailItem label="PostgreSQL" value={node.postgresInstalled ? node.postgresVersion || "Installed" : "Not installed"} />
              <DetailItem label="Data Directory" value={node.postgresDataInitialized ? "Initialized" : "Not initialized"} />
              <DetailItem label="Service Location" value={node.serviceLocation || "-"} />
              <DetailItem label="Docker Available" value={node.dockerAvailable ? "Yes" : "No"} />
            </DetailSection>
          </div>

          <DetailSection title="Metadata & State" icon={<Network className="size-4 text-muted-foreground" />}>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3 w-full">
              <DetailItem label="State" value={node.status} />
              <DetailItem label="State Detail" value={node.statusDetail || "-"} />
              <DetailItem label="Role" value={node.role || "-"} />
              <DetailItem label="Address" value={`${node.address}:${node.port}`} />
              <DetailItem label="Cluster Association" value={node.clusterId || "None"} />
              <DetailItem label="Installation State" value={node.installationState || "-"} />
            </div>
          </DetailSection>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function DetailSection({ title, icon, children }: { title: string; icon?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="space-y-3">
      <h4 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
        {icon}
        {title}
      </h4>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">{children}</div>
    </div>
  );
}

function DetailItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-border p-3 bg-muted/10 shadow-[inset_0_1px_2px_rgba(0,0,0,0.02)]">
      <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{label}</div>
      <div className="mt-1 text-sm font-medium text-foreground break-all">{value}</div>
    </div>
  );
}

function MetricCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <div className="rounded-lg bg-muted/20 border border-border p-4 hover:border-foreground/15 transition-all duration-200">
      <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{label}</div>
      <div className="mt-1.5 text-2xl font-bold tracking-tight text-foreground">{value}</div>
      <div className="mt-1 text-xs text-muted-foreground font-medium">{detail}</div>
    </div>
  );
}

function MetricSparkline({ label, metrics, valueKey }: { label: string; metrics: NodeMetric[]; valueKey: keyof Pick<NodeMetric, "cpuUsagePercent" | "memoryUsagePercent" | "diskUsagePercent"> }) {
  const points = metrics.map((m) => Number(m[valueKey])).filter((value) => Number.isFinite(value));
  const path = sparklinePath(points);
  const latest = points.length ? points[points.length - 1] : 0;

  return (
    <div className="rounded-lg border border-border p-3 bg-muted/10">
      <div className="flex items-center justify-between mb-2">
        <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{label}</span>
        <span className="text-xs font-bold text-foreground">{points.length ? formatPercent(latest) : "No history"}</span>
      </div>
      <svg viewBox="0 0 120 36" className="h-10 w-full text-foreground/80" preserveAspectRatio="none">
        {path ? <path d={path} fill="none" stroke="currentColor" strokeWidth="1.5" /> : <line x1="0" y1="18" x2="120" y2="18" stroke="currentColor" strokeWidth="1" opacity="0.25" />}
      </svg>
      <div className="mt-1.5 text-[9px] text-muted-foreground uppercase tracking-wider font-semibold">Last {points.length} samples</div>
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
