import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "~/lib/api";

interface Node {
  id: string;
  clusterId: string;
  hostname: string;
  role: string;
  status: string;
  address: string;
  port: number;
  labels: Record<string, string>;
  agentVersion: string;
  lastSeen: string;
  createdAt: string;
  updatedAt: string;
  // Phase 2: PostgreSQL installation & health visibility
  postgresInstalled: boolean;
  postgresVersion: string;
  postgresDataInitialized: boolean;
  // Phase 4: human-readable status detail
  statusDetail: string;
  // Phase 2: service location model
  serviceLocation: string;
  dockerAvailable: boolean;
  installationState: string;
  conflictDetails: string;
  agentConnected: boolean;
  agentLatencyMs: number;
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
    refetchInterval: 5000,
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

export function useRejoinNode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (nodeId: string) =>
      api.post<{ node: Node }>("/skylex.v1.NodeService/RejoinNode", { nodeId }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["nodes"] });
    },
  });
}

export function useDeleteNode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (nodeId: string) =>
      api.post("/skylex.v1.NodeService/DeleteNode", { nodeId }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["nodes"] });
    },
  });
}

export function useResolveInstallationConflict() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ nodeId, action }: { nodeId: string; action: "ADOPT" | "PURGE" | "ABORT" }) =>
      api.post<{ node: Node }>("/skylex.v1.NodeService/ResolveInstallationConflict", {
        nodeId,
        action: `RESOLVE_INSTALLATION_CONFLICT_ACTION_${action}`,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["nodes"] });
      qc.invalidateQueries({ queryKey: ["clusters"] });
      qc.invalidateQueries({ queryKey: ["commandLogs"] });
    },
  });
}

export type { Node };
