import { Badge } from "~/components/Badge";

function formatLatency(ms: number) {
  if (!Number.isFinite(ms) || ms <= 0) return "-";
  if (ms < 1000) return `${ms} ms`;
  return `${(ms / 1000).toFixed(1)} s`;
}

export function AgentStatus({ connected, latencyMs }: { connected: boolean; latencyMs: number }) {
  return (
    <div className="flex flex-col gap-1">
      <Badge label={connected ? "Connected" : "Disconnected"} />
      <span className="text-xs text-gray-500 dark:text-gray-400">
        Ping {connected ? formatLatency(latencyMs) : "-"}
      </span>
    </div>
  );
}
