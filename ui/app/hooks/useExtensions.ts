import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "~/lib/api";

export interface Extension {
  name: string;
  label: string;
  description: string;
  enabled: boolean;
  // status: "off" | "pending" | "ready" | "failed"
  status: string;
  error?: string;
  appliedAt?: string;
  updatedAt?: string;
}

export function useExtensions(clusterId: string) {
  return useQuery({
    queryKey: ["extensions", clusterId],
    queryFn: () =>
      api.post<{ extensions: Extension[] }>(
        "/skylex.v1.PostgresManagementService/GetExtensions",
        { clusterId },
      ),
    enabled: !!clusterId,
    refetchInterval: 5000,
  });
}

export function useSetExtension(clusterId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; enabled: boolean }) =>
      api.post<{ extension: Extension }>(
        "/skylex.v1.PostgresManagementService/SetExtension",
        { clusterId, ...input },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["extensions", clusterId] });
    },
  });
}

export function useApplyExtensions(clusterId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api.post<{ extensions: Extension[] }>(
        "/skylex.v1.PostgresManagementService/ApplyExtensions",
        { clusterId },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["extensions", clusterId] });
      qc.invalidateQueries({ queryKey: ["commandLogs"] });
    },
  });
}
