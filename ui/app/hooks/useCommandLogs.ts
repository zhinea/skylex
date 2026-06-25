import { useQuery } from "@tanstack/react-query";
import { api } from "~/lib/api";

export type CommandLogLevel = "info" | "error" | "warn" | "debug";

export interface CommandLog {
  id: string;
  commandId: string;
  nodeId: string;
  hostname: string;
  level: string;
  message: string;
  timestampMs: number;
}

interface Pagination {
  page: number;
  pageSize: number;
  total: number;
}

export interface CommandLogFilter {
  level?: string; // info|warn|error|debug; empty = all
  // windowMs is a relative lookback from "now" (e.g. 3600000 for 1h). It is
  // materialized to an absolute sinceMs at fetch time, NOT here, so the query
  // key stays stable across renders. Putting a live Date.now() in the key
  // changes it every render and causes an infinite refetch loop. 0 = none.
  windowMs?: number;
  sinceMs?: number; // absolute lower bound (custom range); 0 = unbounded
  untilMs?: number; // absolute upper bound (custom range); 0 = unbounded
}

export function useCommandLogs(
  clusterId?: string,
  nodeId?: string,
  commandId?: string,
  filter: CommandLogFilter = {},
  page = 1,
  pageSize = 200,
  refetchInterval: number | false = 5000,
) {
  const { level = "", windowMs = 0, sinceMs = 0, untilMs = 0 } = filter;
  return useQuery({
    queryKey: [
      "commandLogs",
      clusterId || "",
      nodeId || "",
      commandId || "",
      level,
      windowMs,
      sinceMs,
      untilMs,
    ],
    queryFn: () => {
      // Materialize the relative window to an absolute lower bound HERE (per
      // fetch), so the query key above never contains a live timestamp.
      const effectiveSince = windowMs > 0 ? Date.now() - windowMs : sinceMs;
      return api.post<{ logs: CommandLog[]; pagination: Pagination }>(
        "/skylex.v1.NodeService/ListNodeCommandLogs",
        {
          clusterId: clusterId || "",
          nodeId: nodeId || "",
          commandId: commandId || "",
          level,
          sinceMs: effectiveSince,
          untilMs,
          page,
          pageSize,
        },
      );
    },
    enabled: !!(clusterId || nodeId || commandId),
    refetchInterval,
  });
}
