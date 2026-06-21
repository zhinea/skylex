import type { Node } from "~/hooks/useNodes";
import type { Cluster } from "~/hooks/useClusters";

interface ClusterStatsGridProps {
  cluster: Cluster;
  nodes: Node[];
}

export function ClusterStatsGrid({ cluster, nodes }: ClusterStatsGridProps) {
  const primaryNode = nodes.find((n) => n.role === "NODE_ROLE_PRIMARY");

  const toGB = (bytes: number) => {
    if (!bytes) return "0.00";
    return (bytes / (1024 * 1024 * 1024)).toFixed(2);
  };

  const toTB = (bytes: number) => {
    if (!bytes) return "0.00";
    return (bytes / (1024 * 1024 * 1024 * 1024)).toFixed(2);
  };

  const cpuPercent = (primaryNode?.cpuUsagePercent ?? 0).toFixed(2);
  const cores = primaryNode?.cpuCores ?? 1;

  const memUsedGB = primaryNode ? toGB(primaryNode.memoryUsedBytes ?? 0) : "0.00";
  const memTotalGB = primaryNode ? toGB(primaryNode.memoryTotalBytes ?? 0) : "0.00";
  const memPct = primaryNode ? (primaryNode.memoryUsagePercent ?? 0) : 0;

  const diskUsedGB = primaryNode ? toGB(primaryNode.diskUsedBytes ?? 0) : "0.00";
  const diskTotalBytes = primaryNode?.diskTotalBytes ?? 0;
  const diskTotalDisplay = diskTotalBytes >= 1024 * 1024 * 1024 * 1024 
    ? `${toTB(diskTotalBytes)} TB` 
    : `${toGB(diskTotalBytes)} GB`;
  const diskPct = primaryNode ? (primaryNode.diskUsagePercent ?? 0) : 0;

  const agentLatency = primaryNode
    ? primaryNode.agentConnected
      ? `${primaryNode.agentLatencyMs ?? 0} ms`
      : "Disconnected"
    : "N/A";

  const isConnected = primaryNode?.agentConnected ?? false;

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
      {/* CPU */}
      <div className="v-card p-4 rounded-lg">
        <div className="flex items-center justify-between mb-3">
          <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider">
            Compute (CPU)
          </span>
          <span className="material-symbols-outlined text-sm text-neutral-400 cursor-help" title="Primary CPU utilization and core count">info</span>
        </div>
        <div className="flex items-baseline gap-1 mb-2">
          <span className="text-xl font-bold">{cpuPercent}%</span>
          <span className="text-[10px] text-muted-foreground">/ {cores} Cores</span>
        </div>
        <div className="h-8 w-full mt-2">
          <svg className="w-full h-full sparkline-svg" viewBox="0 0 100 20">
            <path d="M0,15 L10,12 L20,18 L30,5 L40,14 L50,10 L60,16 L70,3 L80,12 L90,8 L100,10"></path>
          </svg>
        </div>
      </div>

      {/* Memory */}
      <div className="v-card p-4 rounded-lg">
        <div className="flex items-center justify-between mb-3">
          <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider">
            Memory (RAM)
          </span>
          <span className="material-symbols-outlined text-sm text-neutral-400 cursor-help" title="Memory usage on primary database node">info</span>
        </div>
        <div className="flex items-baseline gap-1 mb-2">
          <span className="text-xl font-bold">{memUsedGB} GB</span>
          <span className="text-[10px] text-muted-foreground">/ {memTotalGB} GB</span>
        </div>
        <div className="mt-4 h-1 w-full bg-neutral-100 dark:bg-neutral-800 rounded-full overflow-hidden">
          <div className="h-full bg-foreground" style={{ width: `${memPct}%` }}></div>
        </div>
      </div>

      {/* Storage */}
      <div className="v-card p-4 rounded-lg">
        <div className="flex items-center justify-between mb-3">
          <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider font-sans">
            Storage
          </span>
          <span className="material-symbols-outlined text-sm text-neutral-400 cursor-help" title="Disk space consumption of PostgreSQL directory">info</span>
        </div>
        <div className="flex items-baseline gap-1 mb-2">
          <span className="text-xl font-bold">{diskUsedGB} GB</span>
          <span className="text-[10px] text-muted-foreground">/ {diskTotalDisplay}</span>
        </div>
        <div className="mt-4 h-1 w-full bg-neutral-100 dark:bg-neutral-800 rounded-full overflow-hidden">
          <div className="h-full bg-foreground" style={{ width: `${diskPct}%` }}></div>
        </div>
      </div>

      {/* Latency */}
      <div className="v-card p-4 rounded-lg">
        <div className="flex items-center justify-between mb-3">
          <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider">
            Latency
          </span>
          <span className="material-symbols-outlined text-sm text-neutral-400 cursor-help" title="Skylex agent heartbeats ping time">info</span>
        </div>
        <div className="flex items-baseline gap-1 mb-2">
          <span className="text-xl font-bold">{agentLatency}</span>
        </div>
        <div className="mt-2 h-8 w-full flex items-end gap-1">
          {isConnected ? (
            <>
              <div className="flex-1 bg-neutral-100 dark:bg-neutral-800 h-4 rounded-t-sm"></div>
              <div className="flex-1 bg-foreground h-6 rounded-t-sm"></div>
              <div className="flex-1 bg-neutral-100 dark:bg-neutral-800 h-3 rounded-t-sm"></div>
              <div className="flex-1 bg-foreground h-5 rounded-t-sm"></div>
              <div className="flex-1 bg-neutral-100 dark:bg-neutral-800 h-4 rounded-t-sm"></div>
            </>
          ) : (
            <>
              <div className="flex-1 bg-neutral-100 dark:bg-neutral-800 h-1 rounded-t-sm"></div>
              <div className="flex-1 bg-neutral-100 dark:bg-neutral-800 h-1 rounded-t-sm"></div>
              <div className="flex-1 bg-neutral-100 dark:bg-neutral-800 h-1 rounded-t-sm"></div>
              <div className="flex-1 bg-neutral-100 dark:bg-neutral-800 h-1 rounded-t-sm"></div>
              <div className="flex-1 bg-neutral-100 dark:bg-neutral-800 h-1 rounded-t-sm"></div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
