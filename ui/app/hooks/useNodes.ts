import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "~/lib/api";

interface Node {
  id: string;
  clusterId: string;
  hostname: string;
  role: string;
  address: string;
  port: number;
  labels: Record<string, string>;
  agentVersion: string;
  lastSeen: string;
  createdAt: string;
  updatedAt: string;
}

interface Pagination {
  page: number;
  pageSize: number;
  total: number;
}

export function useNodes(clusterId?: string, page = 1, pageSize = 50) {
  return useQuery({
    queryKey: ["nodes", clusterId, page, pageSize],
    queryFn: () =>
      api.post<{ nodes: Node[]; pagination: Pagination }>(
        "/skylex.v1.NodeService/ListNodes",
        { clusterId: clusterId || "", page, pageSize },
      ),
  });
}

export function useDrainNode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (nodeId: string) =>
      api.post<{ node: Node }>("/skylex.v1.NodeService/DrainNode", { nodeId }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["nodes"] });
    },
  });
}

export type { Node };