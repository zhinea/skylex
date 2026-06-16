import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "~/lib/api";

interface StorageConfig {
  id: string;
  name: string;
  type: string;
  endpoint: string;
  bucket: string;
  region: string;
  useTls: boolean;
  createdAt: string;
  updatedAt: string;
}

interface Pagination {
  page: number;
  pageSize: number;
  total: number;
}

export function useStorageConfigs(page = 1, pageSize = 20) {
  return useQuery({
    queryKey: ["storage", page, pageSize],
    queryFn: () =>
      api.post<{ storageConfigs: StorageConfig[]; pagination: Pagination }>(
        "/skylex.v1.StorageService/ListStorageConfigs",
        { page, pageSize },
      ),
  });
}

export function useCreateStorageConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: {
      name: string;
      type?: string;
      endpoint: string;
      bucket: string;
      region?: string;
      accessKey: string;
      secretKey: string;
      useTls?: boolean;
    }) =>
      api.post<{ storageConfig: StorageConfig }>(
        "/skylex.v1.StorageService/CreateStorageConfig",
        input,
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["storage"] });
    },
  });
}

export function useDeleteStorageConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<{}>("/skylex.v1.StorageService/DeleteStorageConfig", { id }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["storage"] });
    },
  });
}

export type { StorageConfig };