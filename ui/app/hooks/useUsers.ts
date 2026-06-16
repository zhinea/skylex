import { useQuery } from "@tanstack/react-query";
import { api } from "~/lib/api";

interface User {
  id: string;
  email: string;
  displayName: string;
  role: string;
  createdAt: string;
}

interface Pagination {
  page: number;
  pageSize: number;
  total: number;
}

export function useUsers(page = 1, pageSize = 20) {
  return useQuery({
    queryKey: ["users", page, pageSize],
    queryFn: () =>
      api.post<{ users: User[]; pagination: Pagination }>(
        "/skylex.v1.AuthService/ListUsers",
        { page, pageSize },
      ),
  });
}

export type { User };