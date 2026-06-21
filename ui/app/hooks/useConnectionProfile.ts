import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "~/lib/api";

export interface ConnectionProfile {
  clusterId: string;
  endpointMode: string;
  publicHost: string;
  publicPort: number;
  sslMode: string;
  allowedCidrs: string[];
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
