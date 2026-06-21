import { useState } from "react";
import type { Node } from "~/hooks/useNodes";
import { useNetworkAccess, useUpdateNetworkAccess, useApplyHBA } from "~/hooks/useConnectionProfile";
import { Card, CardHeader, CardTitle, CardContent } from "~/components/ui/card";
import { Button } from "~/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "~/components/ui/table";
import { Network } from "lucide-react";
import { FeatureNote, RoleStatusBadge } from "./ClusterHelpers";
import { PageSpinner } from "~/components/Spinner";

interface NetworkAccessCardProps {
  clusterId: string;
  nodes: Node[];
}

function cidrTextToList(value: string) {
  return value
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean);
}

function cidrListToText(values: string[]) {
  return values.join("\n");
}

export function NetworkAccessCard({ clusterId, nodes }: NetworkAccessCardProps) {
  const { data, isLoading } = useNetworkAccess(clusterId);
  const updateAccess = useUpdateNetworkAccess();
  const applyHBA = useApplyHBA();
  const [editing, setEditing] = useState(false);
  const [applicationCIDRs, setApplicationCIDRs] = useState("");
  const [adminCIDRs, setAdminCIDRs] = useState("");
  const [replicationCIDRs, setReplicationCIDRs] = useState("");
  const [message, setMessage] = useState<string | null>(null);

  const hbaStatuses = data?.hbaStatuses ?? [];
  const nodeNames = new Map(nodes.map((node) => [node.id, node.hostname]));

  function openEditor() {
    setApplicationCIDRs(cidrListToText(data?.allowedApplicationCidrs ?? []));
    setAdminCIDRs(cidrListToText(data?.allowedAdminCidrs ?? []));
    setReplicationCIDRs(cidrListToText(data?.internalReplicationCidrs ?? []));
    setMessage(null);
    setEditing(true);
  }

  function saveAccess() {
    setMessage(null);
    updateAccess.mutate(
      {
        clusterId,
        allowedApplicationCidrs: cidrTextToList(applicationCIDRs),
        allowedAdminCidrs: cidrTextToList(adminCIDRs),
        internalReplicationCidrs: cidrTextToList(replicationCIDRs),
      },
      {
        onSuccess: () => {
          setEditing(false);
          setMessage("Access rules saved. Apply rules to enforce them on nodes.");
        },
        onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to save access allowlists"),
      },
    );
  }

  function apply() {
    setMessage(null);
    applyHBA.mutate(clusterId, {
      onSuccess: () => setMessage("Access rule apply queued for ready nodes."),
      onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to queue access rule apply"),
    });
  }

  if (isLoading) {
    return (
      <Card className="shadow-xs">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground">Network Access</CardTitle>
        </CardHeader>
        <CardContent className="py-6">
          <PageSpinner />
        </CardContent>
      </Card>
    );
  }

  const renderCIDRs = (values: string[]) =>
    values.length === 0 ? (
      <span className="text-xs text-muted-foreground">No IP ranges configured</span>
    ) : (
      <div className="flex flex-wrap gap-1">
        {values.map((cidr) => (
          <code key={cidr} className="rounded border border-border bg-muted/40 px-1.5 py-0.5 font-mono text-[10px] text-foreground">
            {cidr}
          </code>
        ))}
      </div>
    );

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <Network className="size-4 text-muted-foreground" />
          Network Access
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        <FeatureNote title="What this does:">
          Decide which network ranges can connect to PostgreSQL. Skylex writes these rules into PostgreSQL's own access file, called pg_hba.conf or HBA by PostgreSQL.
        </FeatureNote>
        <p className="text-xs text-muted-foreground">
          Think of this as the database door list: if an app server's IP range is not listed here, PostgreSQL will reject it even if the port is open. Firewall and cloud security group rules are still managed outside Skylex.
        </p>

        {editing ? (
          <div className="rounded-lg border border-border p-4 bg-muted/10 space-y-4">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
              <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                App server IP ranges
                <textarea
                  value={applicationCIDRs}
                  onChange={(e) => setApplicationCIDRs(e.target.value)}
                  rows={4}
                  className="mt-1 w-full px-3 py-1.5 text-xs font-mono border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                  placeholder="10.0.0.0/24"
                />
              </label>
              <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                Admin IP ranges
                <textarea
                  value={adminCIDRs}
                  onChange={(e) => setAdminCIDRs(e.target.value)}
                  rows={4}
                  className="mt-1 w-full px-3 py-1.5 text-xs font-mono border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                  placeholder="192.168.1.0/24"
                />
              </label>
              <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                Cluster internal IP ranges
                <textarea
                  value={replicationCIDRs}
                  onChange={(e) => setReplicationCIDRs(e.target.value)}
                  rows={4}
                  className="mt-1 w-full px-3 py-1.5 text-xs font-mono border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                  placeholder="10.10.0.0/24"
                />
              </label>
            </div>
            <p className="text-[10px] text-muted-foreground leading-normal">Use one IP range per line, for example 10.0.0.0/24. If you are unsure, ask your network or cloud provider for the private CIDR range used by your application servers.</p>
            <div className="flex gap-2 pt-2 border-t border-border/40">
              <Button onClick={saveAccess} disabled={updateAccess.isPending} size="sm">
                {updateAccess.isPending ? "Saving..." : "Save Access Rules"}
              </Button>
              <Button variant="outline" onClick={() => setEditing(false)} size="sm">
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3 border border-border/60 p-4 rounded-lg bg-muted/5">
            <div>
              <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">App server IP ranges</div>
              {renderCIDRs(data?.allowedApplicationCidrs ?? [])}
            </div>
            <div>
              <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Admin IP ranges</div>
              {renderCIDRs(data?.allowedAdminCidrs ?? [])}
            </div>
            <div>
              <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Cluster internal IP ranges</div>
              {renderCIDRs(data?.internalReplicationCidrs ?? [])}
            </div>
          </div>
        )}

        <div className="flex flex-wrap items-center gap-2.5">
          {!editing && (
            <Button onClick={openEditor} variant="outline" size="sm">
              Edit Access Rules
            </Button>
          )}
          <Button onClick={apply} disabled={applyHBA.isPending} size="sm">
            {applyHBA.isPending ? "Queueing..." : "Apply Access Rules"}
          </Button>
          {message && <span className="text-xs font-medium text-muted-foreground">{message}</span>}
        </div>

        <div className="space-y-2.5">
          <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Access Rule Apply Status</div>
          {hbaStatuses.length === 0 ? (
            <div className="py-6 text-center border border-dashed rounded-lg border-border/80">
              <p className="text-xs text-muted-foreground">Access rules have not been applied yet.</p>
            </div>
          ) : (
            <div className="overflow-x-auto rounded-lg border border-border">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/30">
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Node</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Status</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Updated</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Error</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {hbaStatuses.map((status) => (
                    <TableRow key={status.nodeId} className="hover:bg-muted/10 border-b border-border/40 last:border-none">
                      <TableCell className="px-4 py-2.5 font-semibold text-foreground">{nodeNames.get(status.nodeId) || status.nodeId.slice(0, 8)}</TableCell>
                      <TableCell className="px-4 py-2.5"><RoleStatusBadge status={status.status} /></TableCell>
                      <TableCell className="text-muted-foreground px-4 py-2.5 text-xs">{status.updatedAt ? new Date(status.updatedAt).toLocaleString() : "-"}</TableCell>
                      <TableCell className="text-destructive px-4 py-2.5 text-xs font-semibold">{status.error || "-"}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
