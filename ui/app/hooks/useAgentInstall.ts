import { useState } from "react";
import { api, ApiError } from "~/lib/api";

export interface InstallCommandData {
  script_url: string;
  server_addr: string;
  token: string;
}

export function useAgentInstallCommand() {
  const [data, setData] = useState<InstallCommandData | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const generate = async () => {
    setIsLoading(true);
    setError(null);
    try {
      const resp = (await api.post("/skylex.v1.AuthService/GetAgentInstallCommand", {})) as {
        script_url: string;
        server_addr: string;
        token: string;
      };

      setData({ script_url: resp.script_url, server_addr: resp.server_addr, token: resp.token });
    } catch (err) {
      const message =
        err instanceof ApiError
          ? err.message
          : err instanceof Error
            ? err.message
            : "Failed to load install command";
      setError(message);
    } finally {
      setIsLoading(false);
    }
  };

  return {
    data,
    isLoading,
    error,
    generate,
  };
}