import { useEffect, useRef, useState } from "react";
import { useCommandLogs, type CommandLog } from "./useCommandLogs";

const API_BASE = "/api";
const MAX_LOGS = 1000;

export type LogStreamState = "connecting" | "live" | "polling" | "closed";

function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("skylex_token");
}

interface StreamArgs {
  clusterId?: string;
  nodeId?: string;
  commandId?: string;
}

/**
 * useCommandLogStream subscribes to the live SSE command-log stream. It reads the
 * stream with fetch + ReadableStream (not EventSource) so the Bearer token can be
 * sent as a header instead of leaking in the URL.
 *
 * The server replays a recent backlog on connect, then pushes live entries, so a
 * freshly opened view is never blank. If the stream can't be established (network,
 * proxy, server without SSE) it transparently falls back to the existing 5s
 * polling query, so logs always render.
 */
export function useCommandLogStream({ clusterId, nodeId, commandId }: StreamArgs) {
  const [logs, setLogs] = useState<CommandLog[]>([]);
  const [state, setState] = useState<LogStreamState>("connecting");
  const seenIds = useRef<Set<string>>(new Set());
  const enabled = !!(clusterId || nodeId || commandId);

  // Polling fallback — only fetches when the stream isn't live.
  const fallback = useCommandLogs(
    state === "polling" ? clusterId : undefined,
    state === "polling" ? nodeId : undefined,
    state === "polling" ? commandId : undefined,
  );

  function appendLogs(incoming: CommandLog[]) {
    if (incoming.length === 0) return;
    setLogs((prev) => {
      const merged = prev.slice();
      for (const log of incoming) {
        if (seenIds.current.has(log.id)) continue;
        seenIds.current.add(log.id);
        merged.push(log);
      }
      merged.sort((a, b) => Number(a.timestampMs) - Number(b.timestampMs));
      if (merged.length > MAX_LOGS) {
        const dropped = merged.splice(0, merged.length - MAX_LOGS);
        for (const d of dropped) seenIds.current.delete(d.id);
      }
      return merged;
    });
  }

  // Merge polling results into the unified buffer when in fallback mode.
  useEffect(() => {
    if (state === "polling" && fallback.data?.logs) {
      appendLogs(fallback.data.logs);
    }
  }, [state, fallback.data]);

  useEffect(() => {
    if (!enabled || typeof window === "undefined") return;

    seenIds.current = new Set();
    setLogs([]);
    setState("connecting");

    const controller = new AbortController();
    let cancelled = false;

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

          // SSE frames are separated by a blank line.
          let sep: number;
          while ((sep = buffer.indexOf("\n\n")) !== -1) {
            const frame = buffer.slice(0, sep);
            buffer = buffer.slice(sep + 2);
            const line = frame.split("\n").find((l) => l.startsWith("data:"));
            if (!line) continue; // heartbeat/comment
            try {
              appendLogs([JSON.parse(line.slice(5).trim()) as CommandLog]);
            } catch {
              // ignore malformed frame, keep stream alive
            }
          }
        }
        // Stream ended (server shutdown / network) — fall back to polling.
        if (!cancelled) setState("polling");
      } catch {
        if (!cancelled) setState("polling");
      }
    }

    run();

    return () => {
      cancelled = true;
      controller.abort();
      setState("closed");
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clusterId, nodeId, commandId, enabled]);

  return { logs, state };
}
