import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "~/lib/api";

export interface AgentToken {
  id: string;
  name: string;
  role: string;
  expires_at: string | null;
  created_at: string;
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
mutationFn: (body: { name: string; role?: string; expires_at?: string }) =>
      api.post<{ agent_token: AgentToken; token: string }>(
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
