import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "~/lib/api";

export interface PostgresRole {
  id: string;
  clusterId: string;
  roleName: string;
  roleKind: string;
  passwordVersion: number;
  expiresAt?: string;
  status: string;
  createdAt?: string;
  updatedAt?: string;
}

export function usePostgresRoles(clusterId: string) {
  return useQuery({
    queryKey: ["postgresRoles", clusterId],
    queryFn: () =>
      api.post<{ roles: PostgresRole[] }>(
        "/skylex.v1.PostgresManagementService/ListRoles",
        { clusterId },
      ),
    enabled: !!clusterId,
    refetchInterval: 5000,
  });
}

export function useCreatePostgresRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { clusterId: string; roleName: string; roleKind: string }) =>
      api.post<{ role: PostgresRole; oneTimePassword: string }>(
        "/skylex.v1.PostgresManagementService/CreateRole",
        input,
      ),
    onSuccess: (_, input) => {
      qc.invalidateQueries({ queryKey: ["postgresRoles", input.clusterId] });
      qc.invalidateQueries({ queryKey: ["commandLogs"] });
    },
  });
}

export function useRotatePostgresRolePassword(clusterId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (roleId: string) =>
      api.post<{ role: PostgresRole; oneTimePassword: string }>(
        "/skylex.v1.PostgresManagementService/RotateRolePassword",
        { roleId },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["postgresRoles", clusterId] });
      qc.invalidateQueries({ queryKey: ["commandLogs"] });
    },
  });
}

export function useDeletePostgresRole(clusterId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (roleId: string) =>
      api.post<{}>("/skylex.v1.PostgresManagementService/DeleteRole", { roleId }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["postgresRoles", clusterId] });
      qc.invalidateQueries({ queryKey: ["commandLogs"] });
    },
  });
}
