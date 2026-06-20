import { useEffect, useState } from "react";
import { Modal } from "./Modal";
import { useAgentInstallCommand } from "~/hooks/useAgentInstall";
import { useNodes } from "~/hooks/useNodes";

interface InstallAgentModalProps {
  open: boolean;
  onClose: () => void;
  mode?: "install" | "reconnect";
  hostname?: string;
}

export function InstallAgentModal({ open, onClose, mode = "install", hostname }: InstallAgentModalProps) {
  const { data, isLoading, error, generate } = useAgentInstallCommand();
  const { data: nodesData } = useNodes(undefined, 1, 100);
  const [docker, setDocker] = useState(false);
  const [withDockerEngine, setWithDockerEngine] = useState(true);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (open) {
      generate();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const initialCount = nodesData?.pagination?.total || 0;
  const currentCount = nodesData?.nodes?.length || 0;
  const newNodesSeen = currentCount > initialCount;

  const command = buildCommand(data?.scriptUrl, data?.serverAddr, data?.token, docker, withDockerEngine);

  const handleCopy = async () => {
    if (typeof window === "undefined" || !command) return;
    try {
      await window.navigator.clipboard.writeText(command);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  };

  const isReconnect = mode === "reconnect";

  return (
    <Modal open={open} title={isReconnect ? "Reconnect Agent" : "Install Agent"} onClose={onClose}>
      <div className="space-y-5">
        <p className="text-sm text-gray-600 dark:text-gray-300">
          {isReconnect
            ? `Run the one-liner below on ${hostname || "the disconnected host"} to reconnect its Skylex agent.`
            : "Run the one-liner below on a Linux database server to install and register the Skylex agent."}
        </p>

        <div className="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg p-4 text-sm text-amber-800 dark:text-amber-300">
          <strong>Prerequisites:</strong> Linux server, root or sudo access, outbound gRPC to{" "}
          <code className="font-mono">{data?.serverAddr || "skylex-server:9090"}</code>.
        </div>

        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => setDocker(false)}
            className={`px-3 py-1.5 text-sm rounded-md border ${
              !docker
                ? "bg-blue-50 border-blue-200 text-blue-700 dark:bg-blue-900/30 dark:border-blue-800 dark:text-blue-400"
                : "border-gray-300 text-gray-700 dark:border-gray-600 dark:text-gray-300"
            }`}
          >
            systemd
          </button>
          <button
            type="button"
            onClick={() => setDocker(true)}
            className={`px-3 py-1.5 text-sm rounded-md border ${
              docker
                ? "bg-blue-50 border-blue-200 text-blue-700 dark:bg-blue-900/30 dark:border-blue-800 dark:text-blue-400"
                : "border-gray-300 text-gray-700 dark:border-gray-600 dark:text-gray-300"
            }`}
          >
            Docker
          </button>
        </div>

        {!docker && (
          <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input
              type="checkbox"
              checked={withDockerEngine}
              onChange={(e) => setWithDockerEngine(e.target.checked)}
              className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            Pre-install Docker Engine and add the agent user to the docker group
          </label>
        )}

        {isLoading && <p className="text-sm text-gray-500">Generating install command...</p>}
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

        {command && !isLoading && (
          <div className="space-y-2">
            <label className="block text-xs font-medium text-gray-700 dark:text-gray-300">
              Copy this command and run it on {isReconnect ? "the disconnected server" : "the target server"}
            </label>
            <div className="relative">
              <pre className="bg-gray-900 text-gray-100 text-xs p-3 rounded-lg overflow-x-auto whitespace-pre-wrap break-all pr-20">
                {command}
              </pre>
              <button
                type="button"
                onClick={handleCopy}
                className="absolute top-2 right-2 px-2 py-1 text-xs bg-gray-700 hover:bg-gray-600 text-white rounded"
              >
                {copied ? "Copied" : "Copy"}
              </button>
            </div>
            <p className="text-xs text-gray-500 dark:text-gray-400">
              The token is shown only once. Keep it secret and revoke it if it is compromised.
            </p>
          </div>
        )}

        <div className="pt-4 border-t border-gray-200 dark:border-gray-700">
          <div className="flex items-center justify-between text-sm">
            <span className="text-gray-600 dark:text-gray-300">
              Registered nodes: <strong>{currentCount}</strong>
            </span>
            {newNodesSeen && (
              <span className="text-green-600 dark:text-green-400 font-medium">
                New node detected
              </span>
            )}
          </div>
          <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
            {isReconnect
              ? "The Nodes list refreshes automatically. It may take a few seconds for the agent to reconnect."
              : "The Nodes list refreshes automatically. It may take a few seconds for the agent to appear."}
          </p>
        </div>
      </div>
    </Modal>
  );
}

function buildCommand(
  scriptUrl: string | undefined,
  serverAddr: string | undefined,
  token: string | undefined,
  docker: boolean,
  withDockerEngine: boolean,
): string {
  if (!scriptUrl || !serverAddr || !token) return "";

  const parts = [
    `curl -fsSL "${scriptUrl}" | sudo bash -s --`,
    `--server "${serverAddr}"`,
    `--token "${token}"`,
  ];

  if (docker) {
    parts.push("--docker");
  } else if (withDockerEngine) {
    parts.push("--with-docker-engine");
  }

  return parts.join(" ");
}
