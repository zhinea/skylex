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
  tlsCertFile?: string;
  tlsKeyFile?: string;
  tlsCaFile?: string;
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
  warnings?: string[];
  tlsConfig?: TLSConfig;
  tlsStatuses?: TLSApplyStatus[];
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

export interface TLSApplyStatus {
  clusterId: string;
  nodeId: string;
  commandId: string;
  requestedTlsMode: string;
  status: string;
  error: string;
  tlsActive: boolean;
  appliedAt?: string;
  updatedAt?: string;
}

export interface TLSConfig {
  clusterId: string;
  tlsMode: string;
  certFile: string;
  keyFile: string;
  caFile: string;
  statuses: TLSApplyStatus[];
  warnings: string[];
  caGenerated: boolean;
  caCreatedAt?: string;
}

export function useTLSConfig(clusterId: string) {
  return useQuery({
    queryKey: ["tlsConfig", clusterId],
    queryFn: () =>
      api.post<{ config: TLSConfig }>(
        "/skylex.v1.PostgresManagementService/GetTLSConfig",
        { clusterId },
      ),
    enabled: !!clusterId,
    refetchInterval: 5000,
  });
}

export function useUpdateTLSConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: {
      clusterId: string;
      tlsMode: string;
      certFile: string;
      keyFile: string;
      caFile: string;
    }) =>
      api.post<{ config: TLSConfig }>(
        "/skylex.v1.PostgresManagementService/UpdateTLSConfig",
        input,
      ),
    onSuccess: (_, input) => {
      qc.invalidateQueries({ queryKey: ["tlsConfig", input.clusterId] });
      qc.invalidateQueries({ queryKey: ["connectionProfile", input.clusterId] });
    },
  });
}

export function useGenerateTLSCA() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (clusterId: string) =>
      api.post<{ config: TLSConfig; caCertPem: string }>(
        "/skylex.v1.PostgresManagementService/GenerateTLSCA",
        { clusterId },
      ),
    onSuccess: (_, clusterId) => {
      qc.invalidateQueries({ queryKey: ["tlsConfig", clusterId] });
      qc.invalidateQueries({ queryKey: ["connectionProfile", clusterId] });
    },
  });
}

export function useTLSCACert(clusterId: string, enabled: boolean) {
  return useQuery({
    queryKey: ["tlsCACert", clusterId],
    queryFn: () =>
      api.post<{ caCertPem: string }>(
        "/skylex.v1.PostgresManagementService/GetTLSCACert",
        { clusterId },
      ),
    enabled: !!clusterId && enabled,
  });
}

export function useApplyTLS() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (clusterId: string) =>
      api.post<{ statuses: TLSApplyStatus[] }>(
        "/skylex.v1.PostgresManagementService/ApplyTLS",
        { clusterId },
      ),
    onSuccess: (_, clusterId) => {
      qc.invalidateQueries({ queryKey: ["tlsConfig", clusterId] });
      qc.invalidateQueries({ queryKey: ["connectionProfile", clusterId] });
      qc.invalidateQueries({ queryKey: ["commandLogs"] });
    },
  });
}
