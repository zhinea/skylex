import { useState } from "react";
import { api, ApiError } from "~/lib/api";

export interface InstallCommandData {
  serverAddr: string;
  token: string;
}

export function useAgentInstallCommand() {
  const [data, setData] = useState<InstallCommandData | null>(null);
  const [version, setVersion] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const generate = async () => {
    setIsLoading(true);
    setError(null);
    try {
      const resp = (await api.post("/skylex.v1.AuthService/GetAgentInstallCommand", {})) as {
        serverAddr: string;
        token: string;
      };

      let ver: string | null = null;
      try {
        const res = await fetch("/version");
        if (res.ok) {
          ver = (await res.text()).trim() || null;
        }
      } catch {
        // version endpoint is optional for the command display
      }

      setData({ serverAddr: resp.serverAddr, token: resp.token });
      setVersion(ver);
    } catch (err) {
      const message = err instanceof ApiError ? err.message : err instanceof Error ? err.message : "Failed to load install command";
      setError(message);
    } finally {
      setIsLoading(false);
    }
  };

  return {
    data,
    version,
    isLoading,
    error,
    generate,
  };
}

export function useScriptUrl() {
  const [url, setUrl] = useState<string>("");
  if (typeof window !== "undefined" && url === "") {
    setUrl(`${window.location.origin}/install.sh`);
  }
  return url;
}
