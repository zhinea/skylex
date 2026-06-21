import { useEffect, useMemo, useRef, useState } from "react";
import { useParams, Link } from "react-router";
import { useCluster } from "~/hooks/useClusters";
import { useNodes } from "~/hooks/useNodes";
import { useCommandLogs, type CommandLog } from "~/hooks/useCommandLogs";
import { useClusterSettings, useUpdateClusterSettings } from "~/hooks/useClusterSettings";
import { useConnectionProfile, useUpdateConnectionProfile } from "~/hooks/useConnectionProfile";
import { useRestartNode } from "~/hooks/useClusters";
import { useRejoinNode, useResolveInstallationConflict } from "~/hooks/useNodes";
import { Badge } from "~/components/Badge";
import { Card } from "~/components/Card";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { PageSpinner } from "~/components/Spinner";
import { SettingInput, curatedSettings, validateSettingValue } from "~/components/SettingInput";
import { AgentStatus } from "~/components/AgentStatus";
import type { Node } from "~/hooks/useNodes";
import type { Cluster } from "~/hooks/useClusters";
import { LayoutDashboard, Link2, Settings as SettingsIcon, ShieldAlert } from "lucide-react";

function PgStatusBadges({
  installed,
  version,
  dataInitialized,
}: {
  installed: boolean;
  version: string;
  dataInitialized: boolean;
}) {
  if (!installed) {
    return (
      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200">
        not installed
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
        {version || "installed"}
      </span>
      <span
        className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
          dataInitialized
            ? "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200"
            : "bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400"
        }`}
      >
        {dataInitialized ? "data ready" : "not initialized"}
      </span>
    </span>
  );
}

function levelColor(level: string): string {
  switch (level.toLowerCase()) {
    case "error":
      return "text-red-700 dark:text-red-400";
    case "warn":
      return "text-yellow-700 dark:text-yellow-400";
    case "info":
      return "text-blue-700 dark:text-blue-400";
    default:
      return "text-gray-500 dark:text-gray-400";
  }
}

function statusDetailColor(detail: string): string {
  switch (detail) {
    case "healthy":
      return "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400";
    case "running":
      return "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400";
    case "syncing_replica":
      return "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400";
    case "initializing_data_directory":
      return "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400";
    case "installation_conflict":
      return "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400";
    case "waiting_for_postgres":
      return "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400";
    case "stopped":
      return "bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400";
    default:
      return "bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400";
  }
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <button
      onClick={handleCopy}
      title="Copy to clipboard"
      className="ml-2 inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-300 dark:hover:bg-gray-600 transition-colors"
    >
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

function ConnectionRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start gap-2 py-2 border-b border-gray-100 dark:border-gray-800 last:border-b-0">
      <dt className="w-36 shrink-0 text-xs text-gray-500 dark:text-gray-400 pt-0.5">{label}</dt>
      <dd className="flex-1 min-w-0 flex items-start gap-1">
        <code className="text-xs font-mono text-gray-900 dark:text-gray-100 break-all">{value}</code>
        <CopyButton text={value} />
      </dd>
    </div>
  );
}

function PostgreSQLConnectionCard({ clusterId, nodes, cluster }: { clusterId: string; nodes: Node[]; cluster: Cluster }) {
  // Only show when nodes are assigned
  if (nodes.length === 0) return null;

  const { data: profileData, isLoading: profileLoading } = useConnectionProfile(clusterId);
  const updateProfile = useUpdateConnectionProfile();

  // Edit state
  const [editing, setEditing] = useState(false);
  const [editMode, setEditMode] = useState("direct_primary");
  const [editPublicHost, setEditPublicHost] = useState("");
  const [editPublicPort, setEditPublicPort] = useState(5432);
  const [editSSLMode, setEditSSLMode] = useState("prefer");
  const [editCIDRs, setEditCIDRs] = useState("");
  const [saveError, setSaveError] = useState<string | null>(null);
  const [savedOk, setSavedOk] = useState(false);

  const profile = profileData?.profile;
  const primaryEndpoint = profileData?.primaryEndpoint;
  const replicaEndpoints = profileData?.replicaEndpoints ?? [];
  const effectiveHost = profileData?.effectiveHost ?? "";
  const effectivePort = profileData?.effectivePort ?? 0;

  // Fallback to node data when profile API hasn't loaded yet
  const fallbackPrimary = nodes.find(
    (n) => n.role === "NODE_ROLE_PRIMARY" && n.postgresInstalled && n.postgresDataInitialized,
  );
  const fallbackReplicas = nodes.filter(
    (n) => n.role === "NODE_ROLE_REPLICA" && n.postgresInstalled && n.postgresDataInitialized,
  );

  const serviceLocation =
    cluster.serviceLocation === "SERVICE_LOCATION_DOCKER" ||
    cluster.config?.serviceLocation === "SERVICE_LOCATION_DOCKER"
      ? "Dockerized"
      : "Native";

  // Determine the endpoint to display
  const displayHost = effectiveHost || (fallbackPrimary ? (fallbackPrimary.address || fallbackPrimary.hostname) : "");
  const displayPort = effectivePort || (fallbackPrimary?.port ?? 5432);
  const sslMode = profile?.sslMode ?? "prefer";

  const isPrimaryReady = !!primaryEndpoint || !!fallbackPrimary;

  function handleEditOpen() {
    setEditMode(profile?.endpointMode ?? "direct_primary");
    setEditPublicHost(profile?.publicHost ?? "");
    setEditPublicPort(profile?.publicPort ?? 5432);
    setEditSSLMode(profile?.sslMode ?? "prefer");
    setEditCIDRs((profile?.allowedCidrs ?? []).join(", "));
    setSaveError(null);
    setSavedOk(false);
    setEditing(true);
  }

  function handleSave() {
    setSaveError(null);
    const cidrs = editCIDRs
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    updateProfile.mutate(
      {
        clusterId,
        endpointMode: editMode,
        publicHost: editPublicHost,
        publicPort: editPublicPort,
        sslMode: editSSLMode,
        allowedCidrs: cidrs,
      },
      {
        onSuccess: () => {
          setSavedOk(true);
          setEditing(false);
          setTimeout(() => setSavedOk(false), 3000);
        },
        onError: (err) => {
          setSaveError(err instanceof Error ? err.message : "Failed to save profile");
        },
      },
    );
  }

  if (profileLoading) {
    return (
      <Card title="PostgreSQL Connection">
        <PageSpinner />
      </Card>
    );
  }

  if (!isPrimaryReady) {
    return (
      <Card title="PostgreSQL Connection">
        <div className="flex items-start gap-3 px-4 py-3 rounded-lg bg-yellow-50 border border-yellow-200 dark:bg-yellow-900/20 dark:border-yellow-700">
          <span className="text-yellow-600 dark:text-yellow-400 mt-0.5 text-base leading-none">⏳</span>
          <div className="text-sm">
            <p className="font-medium text-yellow-900 dark:text-yellow-100">Primary not ready</p>
            <p className="mt-1 text-yellow-700 dark:text-yellow-300">
              Connection details will appear once the primary node has PostgreSQL installed and data
              initialized.
            </p>
          </div>
        </div>
      </Card>
    );
  }

  const endpoint = displayHost ? `${displayHost}:${displayPort}` : "";
  const psqlCommand = displayHost
    ? `psql "host=${displayHost} port=${displayPort} dbname=postgres user=<user> sslmode=${sslMode}"`
    : "";
  const uriTemplate = displayHost
    ? `postgresql://<user>:<password>@${displayHost}:${displayPort}/postgres?sslmode=${sslMode}`
    : "";

  const isManualMode = profile?.endpointMode === "manual_stable_endpoint";
  const hasPublicHost = !!(profile?.publicHost);

  return (
    <Card title="PostgreSQL Connection">
      <div className="space-y-4">
        {/* Edit profile button */}
        <div className="flex justify-end gap-2">
          {savedOk && (
            <span className="text-sm text-green-600 dark:text-green-400 self-center">Profile saved.</span>
          )}
          {!editing && (
            <button
              onClick={handleEditOpen}
              className="px-3 py-1.5 text-xs font-medium rounded-lg border border-gray-300 text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800 transition-colors"
            >
              Edit Profile
            </button>
          )}
        </div>

        {/* Edit form */}
        {editing && (
          <div className="rounded-lg border border-blue-200 bg-blue-50 dark:border-blue-800 dark:bg-blue-900/20 px-4 py-4 space-y-3">
            <p className="text-sm font-medium text-blue-900 dark:text-blue-100">Connection Profile</p>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div>
                <label className="block text-xs text-gray-600 dark:text-gray-400 mb-1">Endpoint Mode</label>
                <select
                  value={editMode}
                  onChange={(e) => setEditMode(e.target.value)}
                  className="w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm px-2 py-1.5 text-gray-900 dark:text-white"
                >
                  <option value="direct_primary">Direct Primary (computed from node)</option>
                  <option value="manual_stable_endpoint">Manual Stable Endpoint</option>
                </select>
              </div>

              <div>
                <label className="block text-xs text-gray-600 dark:text-gray-400 mb-1">SSL Mode</label>
                <select
                  value={editSSLMode}
                  onChange={(e) => setEditSSLMode(e.target.value)}
                  className="w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm px-2 py-1.5 text-gray-900 dark:text-white"
                >
                  <option value="prefer">prefer</option>
                  <option value="require">require</option>
                  <option value="disable">disable</option>
                </select>
              </div>

              <div>
                <label className="block text-xs text-gray-600 dark:text-gray-400 mb-1">Public Host</label>
                <input
                  type="text"
                  value={editPublicHost}
                  onChange={(e) => setEditPublicHost(e.target.value)}
                  placeholder="e.g. pg.example.com"
                  className="w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm px-2 py-1.5 text-gray-900 dark:text-white placeholder-gray-400"
                />
              </div>

              <div>
                <label className="block text-xs text-gray-600 dark:text-gray-400 mb-1">Public Port</label>
                <input
                  type="number"
                  min={1}
                  max={65535}
                  value={editPublicPort}
                  onChange={(e) => setEditPublicPort(Number(e.target.value))}
                  className="w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm px-2 py-1.5 text-gray-900 dark:text-white"
                />
              </div>

              <div className="md:col-span-2">
                <label className="block text-xs text-gray-600 dark:text-gray-400 mb-1">
                  Allowed CIDRs <span className="text-gray-400">(comma-separated, e.g. 10.0.0.0/8, 192.168.1.0/24)</span>
                </label>
                <input
                  type="text"
                  value={editCIDRs}
                  onChange={(e) => setEditCIDRs(e.target.value)}
                  placeholder="e.g. 10.0.0.0/8, 0.0.0.0/0"
                  className="w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm px-2 py-1.5 text-gray-900 dark:text-white placeholder-gray-400"
                />
              </div>
            </div>

            {saveError && (
              <p className="text-sm text-red-600 dark:text-red-400">{saveError}</p>
            )}

            <div className="flex gap-2 pt-1">
              <button
                onClick={handleSave}
                disabled={updateProfile.isPending}
                className="px-4 py-1.5 text-sm font-medium bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-colors"
              >
                {updateProfile.isPending ? "Saving..." : "Save"}
              </button>
              <button
                onClick={() => setEditing(false)}
                className="px-4 py-1.5 text-sm font-medium rounded-lg border border-gray-300 text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800 transition-colors"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {/* Warnings */}
        <div className="space-y-2">
          {!isManualMode && (
            <div className="flex items-start gap-2 rounded-lg border border-yellow-200 bg-yellow-50 px-3 py-2 dark:border-yellow-800 dark:bg-yellow-900/20">
              <span className="mt-0.5 shrink-0 text-yellow-600 dark:text-yellow-400">⚠</span>
              <p className="text-xs text-yellow-800 dark:text-yellow-200">
                <span className="font-medium">Direct node endpoint:</span> This endpoint points directly
                to the primary node and may change after a failover. Configure a stable endpoint (VIP, DNS, or
                proxy) via Edit Profile.
              </p>
            </div>
          )}
          {isManualMode && !hasPublicHost && (
            <div className="flex items-start gap-2 rounded-lg border border-yellow-200 bg-yellow-50 px-3 py-2 dark:border-yellow-800 dark:bg-yellow-900/20">
              <span className="mt-0.5 shrink-0 text-yellow-600 dark:text-yellow-400">⚠</span>
              <p className="text-xs text-yellow-800 dark:text-yellow-200">
                <span className="font-medium">Manual stable endpoint mode is active</span> but no public host is
                set. Set a public host via Edit Profile or switch to Direct Primary.
              </p>
            </div>
          )}
          <div className="flex items-start gap-2 rounded-lg border border-blue-200 bg-blue-50 px-3 py-2 dark:border-blue-800 dark:bg-blue-900/20">
            <span className="mt-0.5 shrink-0 text-blue-600 dark:text-blue-400">ℹ</span>
            <p className="text-xs text-blue-800 dark:text-blue-200">
              <span className="font-medium">Firewall:</span> Ensure port {displayPort} is open from your
              application to the PostgreSQL node. Skylex does not manage firewall or security group
              rules.
            </p>
          </div>
        </div>

        {/* Connection details */}
        {endpoint && (
          <dl className="rounded-lg border border-gray-200 dark:border-gray-700 px-3 divide-y-0">
            <ConnectionRow label="Primary Endpoint" value={endpoint} />
            <ConnectionRow label="Default Database" value="postgres" />
            <ConnectionRow label="SSL Mode" value={sslMode} />
            <ConnectionRow label="Service Location" value={serviceLocation} />
            {psqlCommand && <ConnectionRow label="psql Command" value={psqlCommand} />}
            {uriTemplate && <ConnectionRow label="URI Template" value={uriTemplate} />}
          </dl>
        )}

        {endpoint && (
          <p className="text-xs text-gray-500 dark:text-gray-400">
            Replace <code className="font-mono">&lt;user&gt;</code> and{" "}
            <code className="font-mono">&lt;password&gt;</code> with your PostgreSQL credentials.
            Passwords are never stored or shown by Skylex.
          </p>
        )}

        {/* Allowed CIDRs summary */}
        {profile?.allowedCidrs && profile.allowedCidrs.length > 0 && (
          <div>
            <div className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide mb-1">
              Allowed CIDRs
            </div>
            <div className="flex flex-wrap gap-1">
              {profile.allowedCidrs.map((cidr) => (
                <span
                  key={cidr}
                  className="px-2 py-0.5 rounded text-xs font-mono bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300"
                >
                  {cidr}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Replica endpoints */}
        {(replicaEndpoints.length > 0 || fallbackReplicas.length > 0) && (
          <div>
            <div className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide mb-2">
              Replica Endpoints (read-only)
            </div>
            <dl className="rounded-lg border border-gray-200 dark:border-gray-700 px-3 divide-y-0">
              {(replicaEndpoints.length > 0 ? replicaEndpoints : fallbackReplicas.map((n) => ({
                nodeId: n.id,
                hostname: n.hostname,
                host: n.address || n.hostname,
                port: n.port || 5432,
                role: n.role,
              }))).map((ep) => (
                <ConnectionRow
                  key={ep.nodeId}
                  label={ep.hostname}
                  value={`${ep.host}:${ep.port}`}
                />
              ))}
            </dl>
          </div>
        )}
      </div>
    </Card>
  );
}

function InstallationConflictCard({ nodes, onResolve, pending }: {
  nodes: Node[];
  onResolve: (nodeId: string, action: "ADOPT" | "PURGE" | "ABORT") => void;
  pending: boolean;
}) {
  if (nodes.length === 0) return null;

  return (
    <Card title="Native PostgreSQL Conflict">
      <div className="space-y-4">
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 dark:border-red-800 dark:bg-red-900/20">
          <p className="text-sm font-medium text-red-900 dark:text-red-100">
            Existing native PostgreSQL or data was found on {nodes.length} selected node{nodes.length === 1 ? "" : "s"}.
          </p>
          <p className="mt-1 text-sm text-red-700 dark:text-red-200">
            Skylex is paused to avoid unplanned data loss. Choose Use Existing to adopt the detected installation, Remove & Reinstall to purge packages and the configured data directory, or Abort Cluster Creation.
          </p>
        </div>

        {nodes.map((node) => (
          <div key={node.id} className="rounded-lg border border-gray-200 p-4 dark:border-gray-700">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
              <div>
                <div className="font-medium text-gray-900 dark:text-white">{node.hostname}</div>
                <div className="mt-1 text-sm text-gray-600 dark:text-gray-300">
                  {node.conflictDetails || "Existing PostgreSQL installation or data directory content detected."}
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <button
                  onClick={() => onResolve(node.id, "ADOPT")}
                  disabled={pending}
                  className="rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
                >
                  Use Existing
                </button>
                <button
                  onClick={() => onResolve(node.id, "PURGE")}
                  disabled={pending}
                  className="rounded-lg bg-red-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:opacity-50"
                >
                  Remove & Reinstall
                </button>
                <button
                  onClick={() => onResolve(node.id, "ABORT")}
                  disabled={pending}
                  className="rounded-lg border border-gray-300 px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:opacity-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-800"
                >
                  Abort Cluster Creation
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>
    </Card>
  );
}

function SettingsCard({ clusterId }: { clusterId: string }) {
  const { data, isLoading } = useClusterSettings(clusterId);
  const update = useUpdateClusterSettings();
  const [values, setValues] = useState<Record<string, string>>({});
  const [errors, setErrors] = useState<Record<string, string | null>>({});
  const [saved, setSaved] = useState(false);

  const parameters = useMemo(() => data?.settings?.parameters ?? {}, [data?.settings?.parameters]);

  useEffect(() => {
    const next: Record<string, string> = {};
    for (const s of curatedSettings) {
      next[s.key] = parameters[s.key] ?? "";
    }
    setValues(next);
  }, [parameters]);

  const dirty = useMemo(() => {
    let changed = false;
    for (const s of curatedSettings) {
      if ((values[s.key] ?? "") !== (parameters[s.key] ?? "")) {
        changed = true;
      }
    }
    return changed;
  }, [values, parameters]);

  function handleChange(key: string, value: string) {
    setValues((prev) => ({ ...prev, [key]: value }));
    setErrors((prev) => ({ ...prev, [key]: validateSettingValue(key, value) }));
    setSaved(false);
  }

  function handleSave() {
    const nextErrors: Record<string, string | null> = {};
    const payload: Record<string, string> = {};

    for (const s of curatedSettings) {
      const v = values[s.key]?.trim() ?? "";
      const err = validateSettingValue(s.key, v);
      nextErrors[s.key] = err;
      if (!err && v) {
        payload[s.key] = v;
      }
    }

    setErrors(nextErrors);
    if (Object.values(nextErrors).some(Boolean)) {
      return;
    }

    update.mutate(
      { clusterId, settings: payload },
      {
        onSuccess: () => {
          setSaved(true);
          setTimeout(() => setSaved(false), 3000);
        },
      },
    );
  }

  return (
    <Card title="PostgreSQL Settings">
      {isLoading ? (
        <PageSpinner />
      ) : (
        <div className="space-y-4">
          <p className="text-sm text-gray-500 dark:text-gray-400">
            Changes are validated, persisted, and applied to every node in the cluster.
          </p>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {curatedSettings.map((s) => (
              <SettingInput
                key={s.key}
                id={s.key}
                label={s.label}
                type={s.type}
                value={values[s.key] ?? ""}
                onChange={(v) => handleChange(s.key, v)}
                hint={s.hint}
                options={s.options}
                disabled={update.isPending}
              />
            ))}
          </div>
          {curatedSettings.map((s) =>
            errors[s.key] ? (
              <p key={`${s.key}-err`} className="text-sm text-red-600 dark:text-red-400">
                {s.label}: {errors[s.key]}
              </p>
            ) : null,
          )}
          <div className="flex items-center gap-3 pt-2">
            <button
              onClick={handleSave}
              disabled={!dirty || update.isPending}
              className="px-4 py-2 text-sm font-medium bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-colors"
            >
              {update.isPending ? "Saving..." : "Apply Settings"}
            </button>
            {saved && (
              <span className="text-sm text-green-600 dark:text-green-400">Settings queued for all nodes.</span>
            )}
            {update.isError && (
              <span className="text-sm text-red-600 dark:text-red-400">
                {update.error instanceof Error ? update.error.message : "Failed to update settings"}
              </span>
            )}
          </div>
        </div>
      )}
    </Card>
  );
}

function InstallationProgressCard({ nodes, logs }: { nodes: Node[]; logs: CommandLog[] }) {
  const nodeList = nodes;
  const totalNodes = nodeList.length;
  const readyNodes = nodeList.filter((n) => n.postgresInstalled && n.postgresDataInitialized);
  const progressPct = totalNodes > 0 ? Math.round((readyNodes.length / totalNodes) * 100) : 0;
  const tailLogs = logs.slice(-12);

  return (
    <Card title="Installation Progress">
      <div className="space-y-4">
        <div>
          <div className="flex justify-between text-sm mb-1">
            <span className="text-gray-600 dark:text-gray-400">Provisioning</span>
            <span className="text-gray-900 dark:text-white font-medium">
              {readyNodes.length}/{totalNodes} nodes ready
            </span>
          </div>
          <div className="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2.5">
            <div
              className={`h-2.5 rounded-full transition-all duration-500 ${
                progressPct === 100 ? "bg-green-500" : progressPct > 0 ? "bg-blue-500" : "bg-yellow-500"
              }`}
              style={{ width: `${progressPct}%` }}
            />
          </div>
        </div>

        {nodeList.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-400">Node</th>
                  <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-400">Location</th>
                  <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-400">Install State</th>
                </tr>
              </thead>
              <tbody>
                {nodeList.map((n) => (
                  <tr key={n.id} className="border-b border-gray-100 dark:border-gray-800">
                    <td className="px-3 py-2">
                      <div className="text-gray-900 dark:text-white font-medium">{n.hostname}</div>
                      <div className="text-xs text-gray-500 dark:text-gray-400">{n.role}</div>
                    </td>
                    <td className="px-3 py-2 text-gray-700 dark:text-gray-300">
                      {n.serviceLocation === "SERVICE_LOCATION_DOCKER" ? "Dockerized" : "Native"}
                    </td>
                    <td className="px-3 py-2">
                      <PgStatusBadges
                        installed={n.postgresInstalled}
                        version={n.postgresVersion}
                        dataInitialized={n.postgresDataInitialized}
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <div>
          <div className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide mb-2">
            Recent Installation Logs
          </div>
          {tailLogs.length === 0 ? (
            <p className="text-sm text-gray-500 dark:text-gray-400">Logs appear once agents start executing installation commands.</p>
          ) : (
            <div className="max-h-48 overflow-y-auto font-mono text-xs rounded-lg bg-gray-50 dark:bg-gray-900 border border-gray-200 dark:border-gray-700">
              {tailLogs.map((log) => (
                <div key={log.id} className="grid grid-cols-[5rem_7rem_1fr] gap-2 px-3 py-1.5 border-b border-gray-100 dark:border-gray-800 last:border-b-0">
                  <span className="text-gray-500 dark:text-gray-400">{new Date(Number(log.timestampMs)).toLocaleTimeString()}</span>
                  <span className="text-gray-700 dark:text-gray-300 truncate">{log.hostname || log.nodeId?.slice(0, 8) || "-"}</span>
                  <span className={`${levelColor(log.level)} break-all`}>{log.message}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </Card>
  );
}

export default function ClusterDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: clusterData, isLoading: clusterLoading } = useCluster(id || "");
  const { data: nodesData } = useNodes(id || "");
  const { data: logsData } = useCommandLogs(id || "");
  const restartNode = useRestartNode();
  const rejoinNode = useRejoinNode();
  const resolveConflict = useResolveInstallationConflict();
  const logsEndRef = useRef<HTMLDivElement>(null);

  const [actionNodeId, setActionNodeId] = useState<string | null>(null);
  const [actionType, setActionType] = useState<"restart" | "rejoin" | null>(null);
  const [conflictAction, setConflictAction] = useState<{ nodeId: string; action: "PURGE" | "ABORT" } | null>(null);
  const [activeTab, setActiveTab] = useState<"overview" | "connect" | "settings" | "diagnostics">("overview");

  const logs: CommandLog[] = logsData?.logs ?? [];

  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  if (clusterLoading) return <PageSpinner />;

  const cluster = clusterData?.cluster;
  if (!cluster) {
    return (
      <div className="text-center py-12">
        <h3 className="text-lg font-medium text-gray-900 dark:text-white">Cluster not found</h3>
        <Link to="/clusters" className="mt-4 text-blue-600 hover:text-blue-800 text-sm">Back to Clusters</Link>
      </div>
    );
  }

  const nodes = nodesData?.nodes || [];
  const conflictNodes = nodes.filter((n) => n.installationState === "INSTALLATION_STATE_CONFLICT");
  const onlineNodes = nodes.filter((n) => n.postgresInstalled && n.postgresDataInitialized);
  const totalNodes = nodes.length;

  // Compute overall progress
  const progressPct = totalNodes > 0 ? Math.round((onlineNodes.length / totalNodes) * 100) : 0;

  // Find latest error log
  const lastErrorLog = logs.filter((l) => l.level === "error").slice(-1)[0] ?? null;

  // Suggest fix from error message
  const suggestedFix = (() => {
    if (!lastErrorLog) return null;
    const msg = lastErrorLog.message.toLowerCase();
    if (msg.includes("permission denied")) return "Check data directory ownership on the node.";
    if (msg.includes("port") && (msg.includes("in use") || msg.includes("already")))
      return "Stop the existing PostgreSQL process on the node or change the port.";
    if (msg.includes("connection refused"))
      return "Ensure the primary node is running and reachable from the replica.";
    if (msg.includes("not installed") || msg.includes("not found"))
      return "Install PostgreSQL on the node and re-register the agent.";
    return null;
  })();

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <Link to="/clusters" className="text-sm text-muted-foreground hover:text-foreground mb-1 block transition-colors">
            ← Back to Clusters
          </Link>
          <h2 className="text-2xl font-bold tracking-tight text-foreground">{cluster.name}</h2>
        </div>
        <Badge label={cluster.status} />
      </div>

      {/* Sub-menu Tabs */}
      <div className="flex gap-6 border-b border-border mb-6">
        <button
          onClick={() => setActiveTab("overview")}
          className={`flex items-center gap-2 pb-3 text-sm font-medium border-b-2 -mb-[2px] transition-all cursor-pointer ${
            activeTab === "overview"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <LayoutDashboard className="size-4" />
          Overview
        </button>
        <button
          onClick={() => setActiveTab("connect")}
          className={`flex items-center gap-2 pb-3 text-sm font-medium border-b-2 -mb-[2px] transition-all cursor-pointer ${
            activeTab === "connect"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <Link2 className="size-4" />
          Connect
        </button>
        <button
          onClick={() => setActiveTab("settings")}
          className={`flex items-center gap-2 pb-3 text-sm font-medium border-b-2 -mb-[2px] transition-all cursor-pointer ${
            activeTab === "settings"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <SettingsIcon className="size-4" />
          Settings
        </button>
        <button
          onClick={() => setActiveTab("diagnostics")}
          className={`flex items-center gap-2 pb-3 text-sm font-medium border-b-2 -mb-[2px] transition-all cursor-pointer ${
            activeTab === "diagnostics"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <ShieldAlert className="size-4" />
          Diagnostics & Logs
        </button>
      </div>

      {/* Tab Contents */}
      {activeTab === "overview" && (
        <div className="space-y-6">
          {/* Show installation progress if not fully healthy or in CREATING status */}
          {(cluster.status === "CREATING" || progressPct < 100) && (
            <InstallationProgressCard nodes={nodes} logs={logs} />
          )}

          {conflictNodes.length > 0 && (
            <InstallationConflictCard
              nodes={conflictNodes}
              pending={resolveConflict.isPending}
              onResolve={(nodeId, action) => {
                if (action === "ADOPT") {
                  resolveConflict.mutate({ nodeId, action });
                } else {
                  setConflictAction({ nodeId, action });
                }
              }}
            />
          )}

          <Card title={`Nodes (${nodes.length})`}>
            {nodes.length === 0 ? (
              <p className="text-sm text-muted-foreground py-4 text-center">
                No nodes registered for this cluster.
              </p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-border">
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">Hostname</th>
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">Role</th>
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">Address</th>
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">PostgreSQL</th>
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">Agent</th>
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">Version</th>
                      <th className="text-left px-4 py-3 font-medium text-muted-foreground">Last Seen</th>
                    </tr>
                  </thead>
                  <tbody>
                    {nodes.map((n) => (
                      <tr key={n.id} className="border-b border-border/40 last:border-0 hover:bg-muted/10">
                        <td className="px-4 py-3 text-foreground font-medium">{n.hostname}</td>
                        <td className="px-4 py-3"><Badge label={n.role} /></td>
                        <td className="px-4 py-3 text-foreground">{n.address}:{n.port}</td>
                        <td className="px-4 py-3">
                          <PgStatusBadges
                            installed={n.postgresInstalled}
                            version={n.postgresVersion}
                            dataInitialized={n.postgresDataInitialized}
                          />
                        </td>
                        <td className="px-4 py-3">
                          <AgentStatus connected={n.agentConnected} latencyMs={n.agentLatencyMs} />
                        </td>
                        <td className="px-4 py-3 text-foreground">{n.agentVersion || "-"}</td>
                        <td className="px-4 py-3 text-muted-foreground">
                          {n.lastSeen ? new Date(n.lastSeen).toLocaleString() : "-"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <Card title="Configuration">
              <dl className="space-y-2 text-sm">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <dt className="text-muted-foreground">Engine</dt>
                  <dd className="text-foreground font-medium">{cluster.config?.engine || "POSTGRESQL"}</dd>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <dt className="text-muted-foreground">Version</dt>
                  <dd className="text-foreground font-medium">{cluster.config?.version || "16"}</dd>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <dt className="text-muted-foreground">Service Location</dt>
                  <dd className="text-foreground font-medium">
                    {cluster.serviceLocation === "SERVICE_LOCATION_DOCKER" || cluster.config?.serviceLocation === "SERVICE_LOCATION_DOCKER"
                      ? "Dockerized"
                      : "Native"}
                  </dd>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <dt className="text-muted-foreground">Replication</dt>
                  <dd className="text-foreground font-medium">{cluster.config?.replicationMode || "ASYNC"}</dd>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <dt className="text-muted-foreground">Replicas</dt>
                  <dd className="text-foreground font-medium">{cluster.config?.replicaCount || 0}</dd>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <dt className="text-muted-foreground">PITR</dt>
                  <dd className="text-foreground font-medium">{cluster.config?.pitrEnabled ? "Enabled" : "Disabled"}</dd>
                </div>
                <div className="flex justify-between py-1">
                  <dt className="text-muted-foreground">Created</dt>
                  <dd className="text-foreground font-medium">{new Date(cluster.createdAt).toLocaleString()}</dd>
                </div>
              </dl>
            </Card>

            <Card title="Labels">
              {cluster.config?.labels && Object.keys(cluster.config.labels).length > 0 ? (
                <div className="flex flex-wrap gap-2">
                  {Object.entries(cluster.config.labels).map(([k, v]) => (
                    <span key={k} className="px-2.5 py-1 bg-secondary text-secondary-foreground border rounded-md text-xs font-mono">
                      {k}: {v}
                    </span>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">No labels configured</p>
              )}
            </Card>
          </div>
        </div>
      )}

      {activeTab === "connect" && (
        <div className="space-y-6">
          {nodes.length > 0 ? (
            <PostgreSQLConnectionCard clusterId={id || ""} nodes={nodes} cluster={cluster} />
          ) : (
            <p className="text-sm text-muted-foreground py-8 text-center bg-card border rounded-xl">
              No nodes configured. Add nodes to view connection details.
            </p>
          )}
        </div>
      )}

      {activeTab === "settings" && (
        <div className="space-y-6">
          <SettingsCard clusterId={id || ""} />
        </div>
      )}

      {activeTab === "diagnostics" && (
        <div className="space-y-6">
          <Card title="Diagnostics">
            <div className="space-y-4">
              {/* Overall progress bar */}
              <div>
                <div className="flex justify-between text-sm mb-1">
                  <span className="text-muted-foreground">Cluster Progress</span>
                  <span className="text-foreground font-medium">
                    {onlineNodes.length}/{totalNodes} nodes healthy
                  </span>
                </div>
                <div className="w-full bg-muted rounded-full h-2">
                  <div
                    className={`h-2 rounded-full transition-all duration-500 ${
                      progressPct === 100
                        ? "bg-green-500"
                        : progressPct > 50
                          ? "bg-blue-500"
                          : "bg-yellow-500"
                    }`}
                    style={{ width: `${progressPct}%` }}
                  />
                </div>
              </div>

              {/* Last error + suggested fix */}
              {lastErrorLog && (
                <div className="flex items-start gap-3 px-4 py-3 rounded-md bg-destructive/10 border border-destructive/20">
                  <span className="text-destructive font-bold mt-0.5">✗</span>
                  <div className="text-sm">
                    <p className="text-destructive font-medium">Last Error</p>
                    <p className="text-foreground/90 mt-1 font-mono text-xs">
                      [{lastErrorLog.hostname || lastErrorLog.nodeId?.slice(0, 8)}] {lastErrorLog.message}
                    </p>
                    {suggestedFix && (
                      <p className="text-muted-foreground mt-1.5">
                        <span className="font-medium text-foreground">Suggested fix:</span> {suggestedFix}
                      </p>
                    )}
                  </div>
                </div>
              )}

              {/* Per-node status with actions */}
              {nodes.length > 0 && (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-border">
                        <th className="text-left px-3 py-2 font-medium text-muted-foreground">Node</th>
                        <th className="text-left px-3 py-2 font-medium text-muted-foreground">Status</th>
                        <th className="text-left px-3 py-2 font-medium text-muted-foreground">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {nodes.map((n) => (
                        <tr key={n.id} className="border-b border-border/40 last:border-0">
                          <td className="px-3 py-2">
                            <div className="text-foreground font-medium">{n.hostname}</div>
                            <div className="text-xs text-muted-foreground">{n.role}</div>
                          </td>
                          <td className="px-3 py-2">
                            {n.statusDetail ? (
                              <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${statusDetailColor(n.statusDetail)}`}>
                                {n.statusDetail.replace(/_/g, " ")}
                              </span>
                            ) : n.installationState === "INSTALLATION_STATE_CONFLICT" ? (
                              <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${statusDetailColor("installation_conflict")}`}>
                                installation conflict
                              </span>
                            ) : (
                              <Badge label={n.role} />
                            )}
                          </td>
                          <td className="px-3 py-2">
                            <div className="flex items-center gap-3">
                              <button
                                onClick={() => { setActionNodeId(n.id); setActionType("restart"); }}
                                className="text-xs text-primary hover:underline"
                              >
                                Restart
                              </button>
                              {n.role === "NODE_ROLE_REPLICA" && (
                                <button
                                  onClick={() => { setActionNodeId(n.id); setActionType("rejoin"); }}
                                  className="text-xs text-muted-foreground hover:text-foreground hover:underline"
                                >
                                  Re-sync
                                </button>
                              )}
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </Card>

          <Card title="Command Logs">
            {logs.length === 0 ? (
              <p className="text-sm text-muted-foreground py-4 text-center">
                No command logs yet. Logs appear while the agent executes commands.
              </p>
            ) : (
              <div className="overflow-x-auto max-h-[500px] overflow-y-auto font-mono text-xs border rounded-md">
                <table className="w-full">
                  <tbody>
                    {logs.map((log) => (
                      <tr key={log.id} className="border-b border-border/40 last:border-0 hover:bg-muted/5">
                        <td className="px-3 py-2 whitespace-nowrap text-muted-foreground">
                          {new Date(Number(log.timestampMs)).toLocaleTimeString()}
                        </td>
                        <td className="px-3 py-2 whitespace-nowrap text-foreground/80">
                          {log.hostname || log.nodeId?.slice(0, 8) || "-"}
                        </td>
                        <td className="px-3 py-2 whitespace-nowrap">
                          <span className={levelColor(log.level)}>{log.level.toUpperCase()}</span>
                        </td>
                        <td className="px-3 py-2 text-foreground break-all">
                          {log.message}
                        </td>
                      </tr>
                    ))}
                    <tr><td colSpan={4}><div ref={logsEndRef} /></td></tr>
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        </div>
      )}

      {/* Action confirmation dialogs */}
      <ConfirmDialog
        open={!!actionNodeId && actionType === "restart"}
        title="Restart Node"
        message="This will stop and start PostgreSQL on the selected node. The node will be briefly unavailable. Are you sure?"
        confirmLabel="Restart"
        onConfirm={() => {
          if (actionNodeId) {
            restartNode.mutate(actionNodeId);
            setActionNodeId(null);
            setActionType(null);
          }
        }}
        onCancel={() => { setActionNodeId(null); setActionType(null); }}
      />

      <ConfirmDialog
        open={!!actionNodeId && actionType === "rejoin"}
        title="Re-sync Replica"
        message="This will repoint the replica to follow the current primary and restart PostgreSQL. Any divergent data on the replica will be overwritten. Are you sure?"
        confirmLabel="Re-sync"
        onConfirm={() => {
          if (actionNodeId) {
            rejoinNode.mutate(actionNodeId);
            setActionNodeId(null);
            setActionType(null);
          }
        }}
        onCancel={() => { setActionNodeId(null); setActionType(null); }}
      />

      <ConfirmDialog
        open={conflictAction?.action === "PURGE"}
        title="Remove Existing PostgreSQL"
        message="This will stop PostgreSQL, remove native PostgreSQL packages, and delete the configured data directory on this node before reinstalling. This is destructive and cannot be undone."
        confirmLabel="Remove & Reinstall"
        onConfirm={() => {
          if (conflictAction) {
            resolveConflict.mutate({ nodeId: conflictAction.nodeId, action: conflictAction.action });
            setConflictAction(null);
          }
        }}
        onCancel={() => setConflictAction(null)}
      />

      <ConfirmDialog
        open={conflictAction?.action === "ABORT"}
        title="Abort Cluster Creation"
        message="This will mark the cluster creation as failed. Already queued provisioning work will be skipped where possible."
        confirmLabel="Abort Cluster Creation"
        onConfirm={() => {
          if (conflictAction) {
            resolveConflict.mutate({ nodeId: conflictAction.nodeId, action: conflictAction.action });
            setConflictAction(null);
          }
        }}
        onCancel={() => setConflictAction(null)}
      />
    </div>
  );
}
