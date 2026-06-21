import { useState } from "react";
import type { Node } from "~/hooks/useNodes";
import type { Cluster } from "~/hooks/useClusters";
import { useConnectionProfile, useUpdateConnectionProfile } from "~/hooks/useConnectionProfile";
import { Card, CardHeader, CardTitle, CardContent } from "~/components/ui/card";
import { Button } from "~/components/ui/button";
import { Database } from "lucide-react";
import { FeatureNote, ConnectionRow, libpqSSLMode } from "./ClusterHelpers";
import { PageSpinner } from "~/components/Spinner";

interface PostgreSQLConnectionCardProps {
  clusterId: string;
  nodes: Node[];
  cluster: Cluster;
}

export function PostgreSQLConnectionCard({ clusterId, nodes, cluster }: PostgreSQLConnectionCardProps) {
  // Only show when nodes are assigned
  if (nodes.length === 0) return null;

  const { data: profileData, isLoading: profileLoading } = useConnectionProfile(clusterId);
  const updateProfile = useUpdateConnectionProfile();

  // Edit state
  const [editing, setEditing] = useState(false);
  const [editMode, setEditMode] = useState("direct_primary");
  const [editPublicHost, setEditPublicHost] = useState("");
  const [editPublicPort, setEditPublicPort] = useState(5432);
  const [editSSLMode, setEditSSLMode] = useState("disabled");
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
  const sslMode = profile?.sslMode ?? profileData?.tlsConfig?.tlsMode ?? "disabled";
  const connectionSSLMode = libpqSSLMode(sslMode);
  const profileWarnings = profileData?.warnings ?? profileData?.tlsConfig?.warnings ?? [];

  const isPrimaryReady = !!primaryEndpoint || !!fallbackPrimary;

  function handleEditOpen() {
    setEditMode(profile?.endpointMode ?? "direct_primary");
    setEditPublicHost(profile?.publicHost ?? "");
    setEditPublicPort(profile?.publicPort ?? 5432);
    setEditSSLMode(profile?.sslMode ?? "disabled");
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
      <Card className="shadow-xs">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
            <Database className="size-4 text-muted-foreground" />
            PostgreSQL Connection
          </CardTitle>
        </CardHeader>
        <CardContent className="py-6">
          <PageSpinner />
        </CardContent>
      </Card>
    );
  }

  if (!isPrimaryReady) {
    return (
      <Card className="shadow-xs border-amber-200/50 dark:border-amber-800/40">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
            <Database className="size-4 text-muted-foreground" />
            PostgreSQL Connection
          </CardTitle>
        </CardHeader>
        <CardContent className="pt-6">
          <div className="flex items-start gap-3 rounded-lg border border-amber-200/50 bg-amber-50/50 px-4 py-3 dark:border-amber-800/40 dark:bg-amber-950/20">
            <span className="text-amber-600 dark:text-amber-400 mt-0.5 text-base leading-none">⏳</span>
            <div className="text-xs">
              <p className="font-semibold text-amber-900 dark:text-amber-100">Primary not ready</p>
              <p className="mt-1 text-muted-foreground leading-normal">
                Connection details will appear once the primary node has PostgreSQL installed and data
                initialized.
              </p>
            </div>
          </div>
        </CardContent>
      </Card>
    );
  }

  const endpoint = displayHost ? `${displayHost}:${displayPort}` : "";
  const psqlCommand = displayHost
    ? `psql "host=${displayHost} port=${displayPort} dbname=postgres user=<user> sslmode=${connectionSSLMode}"`
    : "";
  const uriTemplate = displayHost
    ? `postgresql://<user>:<password>@${displayHost}:${displayPort}/postgres?sslmode=${connectionSSLMode}`
    : "";

  const isManualMode = profile?.endpointMode === "manual_stable_endpoint";
  const hasPublicHost = !!(profile?.publicHost);

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4 flex flex-row items-center justify-between space-y-0">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <Database className="size-4 text-muted-foreground" />
          PostgreSQL Connection
        </CardTitle>
        <div className="flex items-center gap-2">
          {savedOk && (
            <span className="text-xs font-medium text-emerald-600 dark:text-emerald-400">Profile saved.</span>
          )}
          {!editing && (
            <Button
              onClick={handleEditOpen}
              variant="outline"
              size="xs"
            >
              Edit Profile
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-6 pt-6">
        <FeatureNote title="What this does:">
          Use this section to copy the address that applications need to reach PostgreSQL. The generated examples already include the current encryption setting and the default database name.
        </FeatureNote>

        {/* Edit form */}
        {editing && (
          <div className="rounded-lg border border-border p-4 bg-muted/10 space-y-4">
            <div className="space-y-1">
              <p className="text-xs font-semibold text-foreground">Connection Profile Settings</p>
              <p className="text-[11px] text-muted-foreground leading-normal">
                Direct Primary is easiest for testing. For production apps, use Manual Stable Endpoint with a DNS name, load balancer, or virtual IP that does not change after failover.
              </p>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Endpoint Mode</label>
                <select
                  value={editMode}
                  onChange={(e) => setEditMode(e.target.value)}
                  className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                >
                  <option value="direct_primary" className="bg-popover text-popover-foreground">Direct Primary (computed from node)</option>
                  <option value="manual_stable_endpoint" className="bg-popover text-popover-foreground">Manual Stable Endpoint</option>
                </select>
              </div>

              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Connection encryption</label>
                <select
                  value={editSSLMode}
                  onChange={(e) => setEditSSLMode(e.target.value)}
                  className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                >
                  <option value="disabled" className="bg-popover text-popover-foreground">disabled</option>
                  <option value="prefer" className="bg-popover text-popover-foreground">prefer</option>
                  <option value="required" className="bg-popover text-popover-foreground">required</option>
                </select>
                <p className="text-[10px] text-muted-foreground leading-normal">disabled maps to sslmode=disable, prefer maps to sslmode=prefer, and required maps to require.</p>
              </div>

              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Public Host</label>
                <input
                  type="text"
                  value={editPublicHost}
                  onChange={(e) => setEditPublicHost(e.target.value)}
                  placeholder="pg.example.com"
                  className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                />
              </div>

              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Public Port</label>
                <input
                  type="number"
                  min={1}
                  max={65535}
                  value={editPublicPort}
                  onChange={(e) => setEditPublicPort(Number(e.target.value))}
                  className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                />
              </div>

              <div className="md:col-span-2 space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  Application IP ranges <span className="text-muted-foreground font-normal">(comma-separated)</span>
                </label>
                <input
                  type="text"
                  value={editCIDRs}
                  onChange={(e) => setEditCIDRs(e.target.value)}
                  placeholder="10.0.0.0/8, 0.0.0.0/0"
                  className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                />
                <p className="text-[10px] text-muted-foreground leading-normal">These ranges are also used as app connection allowlists. 0.0.0.0/0 means any IPv4 address and is not recommended for production.</p>
              </div>
            </div>

            {saveError && (
              <p className="text-xs font-semibold text-destructive">{saveError}</p>
            )}

            <div className="flex gap-2 pt-2 border-t border-border/40">
              <Button
                onClick={handleSave}
                disabled={updateProfile.isPending}
                size="sm"
              >
                {updateProfile.isPending ? "Saving..." : "Save Settings"}
              </Button>
              <Button
                variant="outline"
                onClick={() => setEditing(false)}
                size="sm"
              >
                Cancel
              </Button>
            </div>
          </div>
        )}

        {/* Warnings */}
        <div className="space-y-2">
          {!isManualMode && (
            <div className="flex items-start gap-2.5 rounded-lg border border-amber-200/50 bg-amber-50/50 px-3 py-2 dark:border-amber-800/40 dark:bg-amber-950/20 text-xs text-amber-800 dark:text-amber-300 font-medium">
              <span className="mt-0.5 shrink-0 text-amber-600 dark:text-amber-400">⚠️</span>
              <p>
                <span className="font-semibold text-foreground">Direct node endpoint:</span> This endpoint points directly
                to the primary node and may change after a failover. Configure a stable endpoint (VIP, DNS, or
                proxy) via Edit Profile.
              </p>
            </div>
          )}
          {isManualMode && !hasPublicHost && (
            <div className="flex items-start gap-2.5 rounded-lg border border-amber-200/50 bg-amber-50/50 px-3 py-2 dark:border-amber-800/40 dark:bg-amber-950/20 text-xs text-amber-800 dark:text-amber-300 font-medium">
              <span className="mt-0.5 shrink-0 text-amber-600 dark:text-amber-400">⚠️</span>
              <p>
                <span className="font-semibold text-foreground">Manual stable endpoint mode is active</span> but no public host is
                set. Set a public host via Edit Profile or switch to Direct Primary.
              </p>
            </div>
          )}
          <div className="flex items-start gap-2.5 rounded-lg border border-border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
            <span className="mt-0.5 shrink-0 text-muted-foreground">ℹ️</span>
            <p>
              <span className="font-semibold text-foreground">Firewall:</span> Ensure port {displayPort} is open from your
              application to the PostgreSQL node. Skylex does not manage firewall or security group
              rules.
            </p>
          </div>
          {profileWarnings.map((warning) => (
            <div key={warning} className="flex items-start gap-2.5 rounded-lg border border-amber-200/50 bg-amber-50/50 px-3 py-2 dark:border-amber-800/40 dark:bg-amber-950/20 text-xs text-amber-800 dark:text-amber-300 font-medium">
              <span className="mt-0.5 shrink-0 text-amber-600 dark:text-amber-400">⚠️</span>
              <p className="text-xs font-semibold text-amber-800 dark:text-amber-300">{warning}</p>
            </div>
          ))}
        </div>

        {/* Connection details */}
        {endpoint && (
          <dl className="rounded-lg border border-border bg-muted/5">
            <ConnectionRow label="Primary Endpoint" value={endpoint} />
            <ConnectionRow label="Default Database" value="postgres" />
            <ConnectionRow label="Connection encryption" value={sslMode} />
            <ConnectionRow label="Service Location" value={serviceLocation} />
            {psqlCommand && <ConnectionRow label="psql Command" value={psqlCommand} />}
            {uriTemplate && <ConnectionRow label="URI Template" value={uriTemplate} />}
          </dl>
        )}

        {endpoint && (
          <p className="text-[11px] text-muted-foreground leading-normal">
            Replace <code className="font-mono text-foreground bg-muted/40 px-1 rounded">&lt;user&gt;</code> and{" "}
            <code className="font-mono text-foreground bg-muted/40 px-1 rounded">&lt;password&gt;</code> with your PostgreSQL credentials.
            Stored passwords are never displayed; generated passwords appear only once after create or rotate.
          </p>
        )}

        {/* Replica endpoints */}
        {(replicaEndpoints.length > 0 || fallbackReplicas.length > 0) && (
          <div className="space-y-2.5">
            <div className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider">
              Replica Endpoints (read-only)
            </div>
            <dl className="rounded-lg border border-border bg-muted/5">
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
      </CardContent>
    </Card>
  );
}
