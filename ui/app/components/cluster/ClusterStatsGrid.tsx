import type { Node } from "~/hooks/useNodes";
import type { Cluster } from "~/hooks/useClusters";
import { Card, CardContent } from "~/components/ui/card";
import { Cpu, HardDrive, Cpu as MemoryIcon, Activity, Info } from "lucide-react";

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

  const getPercentageColor = (pct: number) => {
    if (pct > 85) return "text-red-500 font-semibold";
    if (pct > 60) return "text-amber-500 font-semibold";
    return "text-foreground font-semibold";
  };

  const cpuText = primaryNode 
    ? `${primaryNode.cpuUsagePercent ?? 0}% / ${primaryNode.cpuCores ?? 1} Cores` 
    : "No primary node specs";

  const memText = primaryNode
    ? `${toGB(primaryNode.memoryUsedBytes ?? 0)} / ${toGB(primaryNode.memoryTotalBytes ?? 0)} GB`
    : "N/A";

  const diskText = primaryNode
    ? `${toGB(primaryNode.diskUsedBytes ?? 0)} / ${toGB(primaryNode.diskTotalBytes ?? 0)} GB`
    : "N/A";

  const agentLatency = primaryNode
    ? primaryNode.agentConnected
      ? `${primaryNode.agentLatencyMs ?? 0} ms`
      : "Disconnected"
    : "N/A";

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      {/* Compute Card */}
      <Card className="shadow-xs hover:border-border/80 transition-colors">
        <CardContent className="p-4 space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-1">
              Compute (CPU)
              <span className="cursor-help" title="Primary CPU utilization and core count">
                <Info className="size-3 text-muted-foreground/60" />
              </span>
            </span>
            <Cpu className="size-3.5 text-muted-foreground/80" />
          </div>
          <div className="space-y-0.5">
            <div className={`text-lg font-bold ${primaryNode ? getPercentageColor(primaryNode.cpuUsagePercent ?? 0) : ""}`}>
              {cpuText}
            </div>
            <p className="text-[10px] text-muted-foreground">Primary node real-time CPU load</p>
          </div>
        </CardContent>
      </Card>

      {/* Memory Card */}
      <Card className="shadow-xs hover:border-border/80 transition-colors">
        <CardContent className="p-4 space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-1">
              Memory (RAM)
              <span className="cursor-help" title="Memory usage on primary database node">
                <Info className="size-3 text-muted-foreground/60" />
              </span>
            </span>
            <MemoryIcon className="size-3.5 text-muted-foreground/80" />
          </div>
          <div className="space-y-0.5">
            <div className={`text-lg font-bold ${primaryNode ? getPercentageColor(primaryNode.memoryUsagePercent ?? 0) : ""}`}>
              {memText}
            </div>
            <p className="text-[10px] text-muted-foreground">
              {primaryNode ? `${primaryNode.memoryUsagePercent?.toFixed(0) ?? 0}% utilized` : "No primary node detected"}
            </p>
          </div>
        </CardContent>
      </Card>

      {/* Storage Card */}
      <Card className="shadow-xs hover:border-border/80 transition-colors">
        <CardContent className="p-4 space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-1">
              Storage (Disk)
              <span className="cursor-help" title="Disk space consumption of PostgreSQL directory">
                <Info className="size-3 text-muted-foreground/60" />
              </span>
            </span>
            <HardDrive className="size-3.5 text-muted-foreground/80" />
          </div>
          <div className="space-y-0.5">
            <div className={`text-lg font-bold ${primaryNode ? getPercentageColor(primaryNode.diskUsagePercent ?? 0) : ""}`}>
              {diskText}
            </div>
            <p className="text-[10px] text-muted-foreground">
              {primaryNode ? `${primaryNode.diskUsagePercent?.toFixed(0) ?? 0}% capacity` : "No primary node detected"}
            </p>
          </div>
        </CardContent>
      </Card>

      {/* Connection Latency Card */}
      <Card className="shadow-xs hover:border-border/80 transition-colors">
        <CardContent className="p-4 space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-1">
              Agent Latency
              <span className="cursor-help" title="Skylex agent heartbeats ping time">
                <Info className="size-3 text-muted-foreground/60" />
              </span>
            </span>
            <Activity className="size-3.5 text-muted-foreground/80" />
          </div>
          <div className="space-y-0.5">
            <div className={`text-lg font-bold ${primaryNode?.agentConnected ? "text-foreground" : "text-destructive"}`}>
              {agentLatency}
            </div>
            <p className="text-[10px] text-muted-foreground">
              {primaryNode?.agentConnected ? "Connected to control plane" : "Agent offline or unreachable"}
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
