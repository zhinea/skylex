import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "~/lib/api";

export interface PostgresDatabase {
  id: string;
  clusterId: string;
  databaseName: string;
  ownerRoleId?: string;
  ownerRoleName?: string;
  status: string;
  createdAt?: string;
  updatedAt?: string;
}

export function usePostgresDatabases(clusterId: string) {
  return useQuery({
    queryKey: ["postgresDatabases", clusterId],
    queryFn: () =>
      api.post<{ databases: PostgresDatabase[] }>(
        "/skylex.v1.PostgresManagementService/ListDatabases",
        { clusterId },
      ),
    enabled: !!clusterId,
    refetchInterval: 5000,
  });
}

export function useCreatePostgresDatabase() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { clusterId: string; databaseName: string; ownerRoleId?: string }) =>
      api.post<{ database: PostgresDatabase }>(
        "/skylex.v1.PostgresManagementService/CreateDatabase",
        input,
      ),
    onSuccess: (_, input) => {
      qc.invalidateQueries({ queryKey: ["postgresDatabases", input.clusterId] });
      qc.invalidateQueries({ queryKey: ["commandLogs"] });
    },
  });
}

export function useDeletePostgresDatabase(clusterId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (databaseId: string) =>
      api.post<{}>("/skylex.v1.PostgresManagementService/DeleteDatabase", { databaseId }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["postgresDatabases", clusterId] });
      qc.invalidateQueries({ queryKey: ["commandLogs"] });
    },
  });
}
