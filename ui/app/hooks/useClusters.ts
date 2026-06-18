import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "~/lib/api";

interface ClusterConfig {
  engine?: string;
  version?: string;
  replicationMode?: string;
  replicaCount?: number;
  storageConfigId?: string;
  pitrEnabled?: boolean;
  labels?: Record<string, string>;
}

interface Cluster {
  id: string;
  name: string;
  config: ClusterConfig;
  status: string;
  createdAt: string;
  updatedAt: string;
}

interface Pagination {
  page: number;
  pageSize: number;
  total: number;
}

export function useClusters(page = 1, pageSize = 20) {
  return useQuery({
    queryKey: ["clusters", page, pageSize],
    queryFn: () =>
      api.post<{ clusters: Cluster[]; pagination: Pagination }>(
        "/skylex.v1.ClusterService/ListClusters",
        { page, pageSize },
      ),
  });
}

export function useCluster(id: string) {
  return useQuery({
    queryKey: ["clusters", id],
    queryFn: () =>
      api.post<{ cluster: Cluster }>("/skylex.v1.ClusterService/GetCluster", { id }),
    enabled: !!id,
  });
}

export function useCreateCluster() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; config: ClusterConfig; nodeIds: string[] }) =>
      api.post<{ cluster: Cluster }>("/skylex.v1.ClusterService/CreateCluster", input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["clusters"] });
    },
  });
}

export function useDeleteCluster() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<{}>("/skylex.v1.ClusterService/DeleteCluster", { id }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["clusters"] });
    },
  });
}

export function useFailoverCluster() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (clusterId: string) =>
      api.post<{ cluster: Cluster }>("/skylex.v1.ClusterService/FailoverCluster", { clusterId }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["clusters"] });
    },
  });
}

export function useRestartNode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (nodeId: string) =>
      api.post<{ node: Node }>("/skylex.v1.ClusterService/RestartNode", { nodeId }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["nodes"] });
      qc.invalidateQueries({ queryKey: ["clusters"] });
    },
  });
}

export type { Cluster, ClusterConfig, Pagination };