import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "~/lib/api";

export interface AgentToken {
  id: string;
  name: string;
  role: string;
  expiresAt: string | null;
  createdAt: string;
}

export function useAgentTokens() {
  return useQuery({
    queryKey: ["agent-tokens"],
    queryFn: () => api.post<{ tokens: AgentToken[] }>("/skylex.v1.AuthService/ListAgentTokens", {}),
  });
}

export function useCreateAgentToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; role?: string; expiresAt?: string }) =>
      api.post<{ agentToken: AgentToken; token: string }>(
        "/skylex.v1.AuthService/CreateAgentToken",
        body,
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agent-tokens"] });
    },
  });
}

export function useDeleteAgentToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<unknown>("/skylex.v1.AuthService/DeleteAgentToken", { id }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agent-tokens"] });
    },
  });
}
