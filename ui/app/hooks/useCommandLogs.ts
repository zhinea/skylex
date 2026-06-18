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

export function useCommandLogs(
  clusterId?: string,
  nodeId?: string,
  commandId?: string,
  page = 1,
  pageSize = 100,
) {
  return useQuery({
    queryKey: ["commandLogs", clusterId || "", nodeId || "", commandId || ""],
    queryFn: () =>
      api.post<{ logs: CommandLog[]; pagination: Pagination }>(
        "/skylex.v1.NodeService/ListNodeCommandLogs",
        {
          clusterId: clusterId || "",
          nodeId: nodeId || "",
          commandId: commandId || "",
          page,
          pageSize,
        },
      ),
    enabled: !!(clusterId || nodeId || commandId),
    refetchInterval: 5000,
  });
}
