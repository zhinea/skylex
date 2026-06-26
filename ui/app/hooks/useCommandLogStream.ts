import { useEffect, useRef, useState } from "react";
import { useCommandLogs, type CommandLog, type CommandLogFilter } from "./useCommandLogs";

const API_BASE = "";

export type LogStreamState = "connecting" | "live" | "polling" | "closed";

function getToken(): string | null {
  return localStorage.getItem("skylex_token");
}

interface StreamArgs {
  clusterId?: string;
  nodeId?: string;
  commandId?: string;
  filter?: CommandLogFilter;
}

/**
 * useCommandLogStream returns command logs with low-latency live updates.
 *
 * Architecture: the polling query (useCommandLogs) is the always-on source of
 * truth — it applies the level/time filters server-side and can never leave the
 * view blank. The SSE connection is purely a *trigger*: when the server pushes a
 * new entry we debounce-refetch the query, so updates feel realtime (~150ms)
 * instead of waiting for the 5s poll. If SSE can't connect or drops, polling
 * simply continues. Filtering stays authoritative on the server, so live and
 * filtered results never diverge.
 */
export function useCommandLogStream({ clusterId, nodeId, commandId, filter }: StreamArgs) {
  const [state, setState] = useState<LogStreamState>("connecting");
  const enabled = !!(clusterId || nodeId || commandId);

  // Source of truth. Poll fast (5s) while SSE is down; slow (20s safety net)
  // once SSE is driving refetches.
  const query = useCommandLogs(
    clusterId,
    nodeId,
    commandId,
    filter,
    1,
    200,
    enabled ? (state === "live" ? 20000 : 5000) : false,
  );

  const refetchRef = useRef(query.refetch);
  refetchRef.current = query.refetch;

  useEffect(() => {
    if (!enabled || typeof window === "undefined") return;

    setState("connecting");
    const controller = new AbortController();
    let cancelled = false;
    let debounce: ReturnType<typeof setTimeout> | null = null;

    const scheduleRefetch = () => {
      if (debounce) return;
      debounce = setTimeout(() => {
        debounce = null;
        refetchRef.current();
      }, 150);
    };

    async function run() {
      const token = getToken();
      const params = new URLSearchParams();
      if (clusterId) params.set("clusterId", clusterId);
      if (nodeId) params.set("nodeId", nodeId);
      if (commandId) params.set("commandId", commandId);

      try {
        const res = await fetch(
          `${API_BASE}/skylex.v1.NodeService/StreamNodeCommandLogs?${params}`,
          {
            method: "GET",
            headers: token ? { Authorization: `Bearer ${token}` } : {},
            signal: controller.signal,
          },
        );

        if (!res.ok || !res.body) {
          if (!cancelled) setState("polling");
          return;
        }
        if (!cancelled) setState("live");

        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";

        while (!cancelled) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });

          let sep: number;
          while ((sep = buffer.indexOf("\n\n")) !== -1) {
            const frame = buffer.slice(0, sep);
            buffer = buffer.slice(sep + 2);
            // Any data frame means new logs exist; refetch through the filtered
            // query rather than trusting the raw push (keeps filters correct).
            if (frame.split("\n").some((l) => l.startsWith("data:"))) {
              scheduleRefetch();
            }
          }
        }
        if (!cancelled) setState("polling");
      } catch {
        if (!cancelled) setState("polling");
      }
    }

    run();

    return () => {
      cancelled = true;
      controller.abort();
      if (debounce) clearTimeout(debounce);
      setState("closed");
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clusterId, nodeId, commandId, enabled]);

  const logs: CommandLog[] = query.data?.logs ?? [];
  return { logs, state, isLoading: query.isLoading };
}
