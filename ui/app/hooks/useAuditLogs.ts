import { useQuery } from "@tanstack/react-query";
import { api } from "~/lib/api";

interface AuditEntry {
  id: string;
  userId: string;
  action: string;
  resource: string;
  detail: string;
  ipAddress: string;
  timestamp: string;
}

interface Pagination {
  page: number;
  pageSize: number;
  total: number;
}

interface AuditResponse {
  entries: AuditEntry[];
  pagination: Pagination;
}

export function useAuditLogs(page = 1, pageSize = 50) {
  return useQuery<AuditResponse>({
    queryKey: ["audit", page, pageSize],
    queryFn: () =>
      api.post<AuditResponse>(
        "/skylex.v1.AuthService/ListAuditLogs",
        { page, pageSize },
      ),
    refetchInterval: 10000,
  });
}

export type { AuditEntry };