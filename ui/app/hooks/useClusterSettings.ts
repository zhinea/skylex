import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "~/lib/api";

interface ClusterSettings {
  parameters: Record<string, string>;
}

export function useClusterSettings(clusterId: string) {
  return useQuery({
    queryKey: ["clusterSettings", clusterId],
    queryFn: () =>
      api.post<{ settings: ClusterSettings }>(
        "/skylex.v1.ClusterService/GetClusterSettings",
        { clusterId },
      ),
    enabled: !!clusterId,
  });
}

export function useUpdateClusterSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { clusterId: string; settings: Record<string, string> }) =>
      api.post<{ cluster: { id: string; name: string } }>(
        "/skylex.v1.ClusterService/UpdateClusterSettings",
        { clusterId: input.clusterId, settings: { parameters: input.settings } },
      ),
    onSuccess: (_, input) => {
      qc.invalidateQueries({ queryKey: ["clusterSettings", input.clusterId] });
    },
  });
}
