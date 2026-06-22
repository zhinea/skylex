import { useState } from "react";
import type { Node } from "~/hooks/useNodes";
import { Card, CardHeader, CardTitle, CardContent } from "~/components/ui/card";
import { Button } from "~/components/ui/button";
import { ShieldAlert } from "lucide-react";

interface AdoptCredentials {
  postgresAdminUser: string;
  postgresAdminPassword: string;
}

interface InstallationConflictCardProps {
  nodes: Node[];
  onResolve: (nodeId: string, action: "ADOPT" | "PURGE" | "ABORT", credentials?: AdoptCredentials) => void;
  pending: boolean;
}

export function InstallationConflictCard({ nodes, onResolve, pending }: InstallationConflictCardProps) {
  const [credentials, setCredentials] = useState<Record<string, AdoptCredentials>>({});
  if (nodes.length === 0) return null;

  function updateCredential(nodeId: string, key: keyof AdoptCredentials, value: string) {
    setCredentials((current) => ({
      ...current,
      [nodeId]: {
        postgresAdminUser: key === "postgresAdminUser" ? value : current[nodeId]?.postgresAdminUser ?? "postgres",
        postgresAdminPassword: key === "postgresAdminPassword" ? value : current[nodeId]?.postgresAdminPassword ?? "",
      },
    }));
  }

  return (
    <Card className="border-destructive/30 shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-destructive flex items-center gap-2">
          <ShieldAlert className="size-4 text-destructive" />
          Native PostgreSQL Conflict
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        <div className="rounded-lg border border-destructive/20 bg-destructive/10 px-4 py-3 text-xs leading-relaxed text-destructive/90">
          <p className="font-semibold text-destructive mb-1">
            Existing native PostgreSQL or data was found on {nodes.length} selected node{nodes.length === 1 ? "" : "s"}.
          </p>
          <p>
            Skylex is paused to avoid unplanned data loss. Choose <strong className="text-foreground">Use Existing</strong> to adopt the detected installation, <strong className="text-foreground">Remove & Reinstall</strong> to purge packages and the configured data directory, or <strong className="text-foreground">Abort Cluster Creation</strong>.
          </p>
        </div>

        {nodes.map((node) => (
          <div key={node.id} className="rounded-lg border border-border p-4 bg-muted/5 space-y-4">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
              <div className="space-y-1">
                <div className="font-semibold text-foreground">{node.hostname}</div>
                <div className="text-xs text-muted-foreground font-mono bg-muted/40 px-2 py-1 rounded border border-border/40">
                  {node.conflictDetails || "Existing PostgreSQL installation or data directory content detected."}
                </div>
                <div className="rounded-lg border border-border/60 bg-background p-3">
                  <p className="mb-3 text-xs font-semibold text-foreground">Existing PostgreSQL root/admin account</p>
                  <div className="flex flex-col gap-3 sm:flex-row">
                    <label className="flex flex-1 flex-col gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                      Username
                      <input
                        value={credentials[node.id]?.postgresAdminUser ?? "postgres"}
                        onChange={(event) => updateCredential(node.id, "postgresAdminUser", event.target.value)}
                        disabled={pending}
                        className="h-8 rounded-md border border-border bg-background px-2 text-xs font-mono text-foreground outline-none focus:border-ring focus:ring-2 focus:ring-ring/30 disabled:opacity-50"
                        autoComplete="username"
                      />
                    </label>
                    <label className="flex flex-1 flex-col gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                      Password
                      <input
                        type="password"
                        value={credentials[node.id]?.postgresAdminPassword ?? ""}
                        onChange={(event) => updateCredential(node.id, "postgresAdminPassword", event.target.value)}
                        disabled={pending}
                        className="h-8 rounded-md border border-border bg-background px-2 text-xs text-foreground outline-none focus:border-ring focus:ring-2 focus:ring-ring/30 disabled:opacity-50"
                        autoComplete="current-password"
                      />
                    </label>
                  </div>
                  <p className="mt-2 text-[11px] leading-relaxed text-muted-foreground">
                    Used once to connect to the existing local PostgreSQL primary and create the Skylex replication role. The password is sent as a short-lived command secret.
                  </p>
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  onClick={() => onResolve(node.id, "ADOPT", credentials[node.id] ?? { postgresAdminUser: "postgres", postgresAdminPassword: "" })}
                  disabled={pending || !(credentials[node.id]?.postgresAdminUser ?? "postgres").trim() || !credentials[node.id]?.postgresAdminPassword}
                  size="sm"
                >
                  Use Existing
                </Button>
                <Button
                  onClick={() => onResolve(node.id, "PURGE")}
                  disabled={pending}
                  variant="destructive"
                  size="sm"
                >
                  Remove & Reinstall
                </Button>
                <Button
                  onClick={() => onResolve(node.id, "ABORT")}
                  disabled={pending}
                  variant="outline"
                  size="sm"
                >
                  Abort Cluster Creation
                </Button>
              </div>
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}
