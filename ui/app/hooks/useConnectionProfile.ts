import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "~/lib/api";

export interface ConnectionProfile {
  clusterId: string;
  endpointMode: string;
  publicHost: string;
  publicPort: number;
  sslMode: string;
  allowedCidrs: string[];
  allowedAdminCidrs?: string[];
  allowedReplicationCidrs?: string[];
  createdAt?: string;
  updatedAt?: string;
}

export interface NodeEndpoint {
  nodeId: string;
  hostname: string;
  host: string;
  port: number;
  role: string;
}

export interface ConnectionProfileData {
  profile: ConnectionProfile;
  primaryEndpoint?: NodeEndpoint;
  replicaEndpoints?: NodeEndpoint[];
  effectiveHost: string;
  effectivePort: number;
}

export function useConnectionProfile(clusterId: string) {
  return useQuery({
    queryKey: ["connectionProfile", clusterId],
    queryFn: () =>
      api.post<ConnectionProfileData>(
        "/skylex.v1.PostgresManagementService/GetConnectionProfile",
        { clusterId },
      ),
    enabled: !!clusterId,
  });
}

export function useUpdateConnectionProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: {
      clusterId: string;
      endpointMode?: string;
      publicHost?: string;
      publicPort?: number;
      sslMode?: string;
      allowedCidrs?: string[];
    }) =>
      api.post<{ profile: ConnectionProfile }>(
        "/skylex.v1.PostgresManagementService/UpdateConnectionProfile",
        input,
      ),
    onSuccess: (_, input) => {
      qc.invalidateQueries({ queryKey: ["connectionProfile", input.clusterId] });
    },
  });
}

export interface HBAApplyStatus {
  clusterId: string;
  nodeId: string;
  commandId: string;
  status: string;
  error: string;
  appliedAt?: string;
  updatedAt?: string;
}

export interface NetworkAccessData {
  allowedApplicationCidrs: string[];
  allowedAdminCidrs: string[];
  internalReplicationCidrs: string[];
  hbaStatuses: HBAApplyStatus[];
}

export function useNetworkAccess(clusterId: string) {
  return useQuery({
    queryKey: ["networkAccess", clusterId],
    queryFn: () =>
      api.post<NetworkAccessData>(
        "/skylex.v1.PostgresManagementService/GetNetworkAccess",
        { clusterId },
      ),
    enabled: !!clusterId,
    refetchInterval: 5000,
  });
}

export function useUpdateNetworkAccess() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: {
      clusterId: string;
      allowedApplicationCidrs: string[];
      allowedAdminCidrs: string[];
      internalReplicationCidrs: string[];
    }) =>
      api.post<Omit<NetworkAccessData, "hbaStatuses">>(
        "/skylex.v1.PostgresManagementService/UpdateNetworkAccess",
        input,
      ),
    onSuccess: (_, input) => {
      qc.invalidateQueries({ queryKey: ["networkAccess", input.clusterId] });
      qc.invalidateQueries({ queryKey: ["connectionProfile", input.clusterId] });
    },
  });
}

export function useApplyHBA() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (clusterId: string) =>
      api.post<{ hbaStatuses: HBAApplyStatus[] }>(
        "/skylex.v1.PostgresManagementService/ApplyHBA",
        { clusterId },
      ),
    onSuccess: (_, clusterId) => {
      qc.invalidateQueries({ queryKey: ["networkAccess", clusterId] });
      qc.invalidateQueries({ queryKey: ["commandLogs"] });
    },
  });
}
