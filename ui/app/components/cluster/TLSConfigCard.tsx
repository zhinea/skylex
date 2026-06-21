import { useState } from "react";
import type { Node } from "~/hooks/useNodes";
import { useTLSConfig, useUpdateTLSConfig, useGenerateTLSCA, useApplyTLS, useTLSCACert } from "~/hooks/useConnectionProfile";
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
import { ShieldAlert } from "lucide-react";
import { FeatureNote, ConnectionRow, RoleStatusBadge } from "./ClusterHelpers";
import { PageSpinner } from "~/components/Spinner";

interface TLSConfigCardProps {
  clusterId: string;
  nodes: Node[];
}

export function TLSConfigCard({ clusterId, nodes }: TLSConfigCardProps) {
  const { data, isLoading } = useTLSConfig(clusterId);
  const updateTLS = useUpdateTLSConfig();
  const generateCA = useGenerateTLSCA();
  const applyTLS = useApplyTLS();
  const [editing, setEditing] = useState(false);
  const [tlsMode, setTLSMode] = useState("disabled");
  const [certFile, setCertFile] = useState("");
  const [keyFile, setKeyFile] = useState("");
  const [caFile, setCAFile] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const [generatedCAPem, setGeneratedCAPem] = useState<string | null>(null);

  const config = data?.config;
  const { data: caData } = useTLSCACert(clusterId, !!config?.caGenerated);
  const caCertPem = generatedCAPem ?? caData?.caCertPem ?? "";
  const statuses = config?.statuses ?? [];
  const warnings = config?.warnings ?? [];
  const nodeNames = new Map(nodes.map((node) => [node.id, node.hostname]));

  function openEditor() {
    setTLSMode(config?.tlsMode ?? "disabled");
    setCertFile(config?.certFile ?? "");
    setKeyFile(config?.keyFile ?? "");
    setCAFile(config?.caFile ?? "");
    setMessage(null);
    setEditing(true);
  }

  function saveTLS() {
    setMessage(null);
    updateTLS.mutate(
      { clusterId, tlsMode, certFile, keyFile, caFile },
      {
        onSuccess: () => {
          setEditing(false);
          setMessage("TLS configuration saved. Apply TLS to enforce it on ready nodes.");
        },
        onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to save TLS configuration"),
      },
    );
  }

  function apply() {
    setMessage(null);
    applyTLS.mutate(clusterId, {
      onSuccess: () => setMessage("TLS apply queued for ready nodes."),
      onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to queue TLS apply"),
    });
  }

  function generate() {
    setMessage(null);
    const continueAnyway = window.confirm(
      "This action will generate or replace the cluster PostgreSQL TLS CA, save TLS mode as prefer, queue TLS apply commands for ready nodes, and create a CA certificate download link. Existing clients that trust the previous CA may need the new certificate. Continue anyway?",
    );
    if (!continueAnyway) return;
    generateCA.mutate(clusterId, {
      onSuccess: (res) => {
        setGeneratedCAPem(res.caCertPem);
        updateTLS.mutate(
          { clusterId, tlsMode: "prefer", certFile: "", keyFile: "", caFile: "" },
          {
            onSuccess: () => {
              applyTLS.mutate(clusterId, {
                onSuccess: () => setMessage("Cluster TLS CA generated, TLS apply queued, and CA certificate is ready to download."),
                onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to queue TLS apply"),
              });
            },
            onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to save TLS configuration"),
          },
        );
      },
      onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to generate TLS CA"),
    });
  }

  function downloadCA() {
    const pem = caCertPem;
    if (!pem || typeof window === "undefined") return;
    const url = window.URL.createObjectURL(new Blob([pem], { type: "application/x-pem-file" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = `skylex-${clusterId}-ca.crt`;
    link.click();
    window.URL.revokeObjectURL(url);
  }

  if (isLoading) {
    return (
      <Card className="shadow-xs">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground">TLS</CardTitle>
        </CardHeader>
        <CardContent className="py-6">
          <PageSpinner />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <ShieldAlert className="size-4 text-muted-foreground" />
          TLS Configuration
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        <FeatureNote title="What this does:">
          TLS encrypts database traffic between apps and PostgreSQL, like HTTPS for your database. It is off by default; generate a CA certificate when you are ready to let clients trust Skylex-managed database certificates.
        </FeatureNote>
        <p className="text-xs text-muted-foreground">
          Beginner path: click Generate CA Cert, confirm, then download the CA certificate and give it to applications that require trusted encrypted database connections. Use manual certificate paths only if your team already manages PostgreSQL certificates outside Skylex.
        </p>

        {warnings.length > 0 && (
          <div className="space-y-2">
            {warnings.map((warning) => (
              <div key={warning} className="rounded-lg border border-amber-200/50 bg-amber-50/50 px-3 py-2 text-xs text-amber-800 dark:border-amber-800/40 dark:bg-amber-950/20 dark:text-amber-300 font-medium">
                {warning}
              </div>
            ))}
          </div>
        )}

        {editing ? (
          <div className="rounded-lg border border-border p-4 bg-muted/10 space-y-4">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">TLS Mode</label>
                <select
                  value={tlsMode}
                  onChange={(e) => setTLSMode(e.target.value)}
                  className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                >
                  <option value="disabled" className="bg-popover text-popover-foreground">disabled</option>
                  <option value="prefer" className="bg-popover text-popover-foreground">prefer</option>
                  <option value="required" className="bg-popover text-popover-foreground">required</option>
                </select>
                <p className="text-[10px] text-muted-foreground leading-normal">disabled = no encryption required, prefer = allow encrypted clients, required = reject unencrypted clients.</p>
              </div>
              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Server Certificate Path (optional)</label>
                <input
                  type="text"
                  value={certFile}
                  onChange={(e) => setCertFile(e.target.value)}
                  placeholder="/etc/skylex/postgres/server.crt"
                  className="w-full px-3 py-1.5 text-xs font-mono border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                />
              </div>
              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Server Key Path (optional)</label>
                <input
                  type="text"
                  value={keyFile}
                  onChange={(e) => setKeyFile(e.target.value)}
                  placeholder="/etc/skylex/postgres/server.key"
                  className="w-full px-3 py-1.5 text-xs font-mono border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                />
              </div>
              <div className="space-y-1.5">
                <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">CA File Path (optional)</label>
                <input
                  type="text"
                  value={caFile}
                  onChange={(e) => setCAFile(e.target.value)}
                  placeholder="/etc/skylex/postgres/ca.crt"
                  className="w-full px-3 py-1.5 text-xs font-mono border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                />
              </div>
            </div>
            <p className="text-[10px] text-muted-foreground leading-normal">Leave certificate and key paths empty unless you already have your own PostgreSQL certificate files on every node. Empty paths tell Skylex to generate and install certificates for you.</p>
            <div className="flex gap-2 pt-2 border-t border-border/40">
              <Button onClick={saveTLS} disabled={updateTLS.isPending} size="sm">
                {updateTLS.isPending ? "Saving..." : "Save TLS"}
              </Button>
              <Button variant="outline" onClick={() => setEditing(false)} size="sm">
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <dl className="rounded-lg border border-border/60 px-3 bg-muted/5">
            <ConnectionRow label="TLS Mode" value={config?.tlsMode ?? "disabled"} />
            <ConnectionRow label="Cluster CA" value={config?.caGenerated ? "Generated" : "Not generated"} />
            <ConnectionRow label="Server Certificate" value={config?.certFile || ((config?.tlsMode ?? "disabled") === "disabled" ? "Not configured" : "Skylex-managed CA-signed per node")} />
            <ConnectionRow label="Server Key" value={config?.keyFile || ((config?.tlsMode ?? "disabled") === "disabled" ? "Not configured" : "Skylex-managed CA-signed per node")} />
            <ConnectionRow label="CA File" value={config?.caFile || ((config?.tlsMode ?? "disabled") !== "disabled" && config?.caGenerated ? "Skylex-managed per node" : "Not configured")} />
          </dl>
        )}

        <div className="flex flex-wrap items-center gap-2.5">
          {!editing && (
            <Button onClick={openEditor} variant="outline" size="sm">
              Edit TLS
            </Button>
          )}
          <Button onClick={generate} disabled={generateCA.isPending || updateTLS.isPending || applyTLS.isPending} variant="outline" size="sm">
            {generateCA.isPending || updateTLS.isPending || applyTLS.isPending ? "Configuring..." : config?.caGenerated ? "Regenerate CA Cert" : "Generate CA Cert"}
          </Button>
          {(config?.caGenerated || generatedCAPem) && (
            <Button onClick={downloadCA} disabled={!caCertPem} variant="outline" size="sm">
              Download CA Cert
            </Button>
          )}
          <Button onClick={apply} disabled={applyTLS.isPending || ((config?.tlsMode ?? "disabled") !== "disabled" && !config?.caGenerated && !config?.certFile)} size="sm">
            {applyTLS.isPending ? "Queueing..." : "Apply TLS"}
          </Button>
          {message && <span className="text-xs font-medium text-muted-foreground">{message}</span>}
        </div>

        <div className="space-y-2.5">
          <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">TLS Apply Status</div>
          {statuses.length === 0 ? (
            <div className="py-6 text-center border border-dashed rounded-lg border-border/80">
              <p className="text-xs text-muted-foreground">TLS has not been applied yet.</p>
            </div>
          ) : (
            <div className="overflow-x-auto rounded-lg border border-border">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/30">
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Node</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Mode</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Status</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Active</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Updated</TableHead>
                    <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4 text-right">Error</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {statuses.map((status) => (
                    <TableRow key={status.nodeId} className="hover:bg-muted/10 border-b border-border/40 last:border-none">
                      <TableCell className="px-4 py-2.5 font-semibold text-foreground">{nodeNames.get(status.nodeId) || status.nodeId.slice(0, 8)}</TableCell>
                      <TableCell className="px-4 py-2.5 text-muted-foreground text-xs font-mono">{status.requestedTlsMode}</TableCell>
                      <TableCell className="px-4 py-2.5"><RoleStatusBadge status={status.status} /></TableCell>
                      <TableCell className="px-4 py-2.5 text-muted-foreground text-xs font-semibold">{status.tlsActive ? "yes" : "no"}</TableCell>
                      <TableCell className="px-4 py-2.5 text-muted-foreground text-xs">{status.updatedAt ? new Date(status.updatedAt).toLocaleString() : "-"}</TableCell>
                      <TableCell className="text-destructive px-4 py-2.5 text-xs font-semibold text-right">{status.error || "-"}</TableCell>
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
