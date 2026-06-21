import { useState } from "react";
import { usePostgresRoles, type PostgresRole } from "~/hooks/usePostgresRoles";
import { usePostgresDatabases, useCreatePostgresDatabase, useDeletePostgresDatabase, type PostgresDatabase } from "~/hooks/usePostgresDatabases";
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
import { Database } from "lucide-react";
import { FeatureNote, CopyButton, RoleStatusBadge, databaseConnectionURI, databasePsqlCommand } from "./ClusterHelpers";
import { PageSpinner } from "~/components/Spinner";

interface ManagedDatabasesCardProps {
  clusterId: string;
  host: string;
  port: number;
  sslMode: string;
  revealedRole: { role: PostgresRole; password: string } | null;
}

export function ManagedDatabasesCard({
  clusterId,
  host,
  port,
  sslMode,
  revealedRole,
}: ManagedDatabasesCardProps) {
  const { data: roleData } = usePostgresRoles(clusterId);
  const { data, isLoading } = usePostgresDatabases(clusterId);
  const createDatabase = useCreatePostgresDatabase();
  const deleteDatabase = useDeletePostgresDatabase(clusterId);
  const [databaseName, setDatabaseName] = useState("");
  const [ownerRoleId, setOwnerRoleId] = useState("");
  const [error, setError] = useState<string | null>(null);

  const roles = (roleData?.roles ?? []).filter((role) => role.status === "ready" && role.roleKind !== "read_only");
  const databases = data?.databases ?? [];
  const revealedDatabasePassword = (database: PostgresDatabase) =>
    revealedRole?.role.roleName === database.ownerRoleName ? revealedRole?.password : undefined;

  function handleCreate() {
    setError(null);
    createDatabase.mutate(
      { clusterId, databaseName: databaseName.trim(), ownerRoleId: ownerRoleId || undefined },
      {
        onSuccess: () => {
          setDatabaseName("");
          setOwnerRoleId("");
        },
        onError: (err) => setError(err instanceof Error ? err.message : "Failed to create database"),
      },
    );
  }

  function handleDelete(database: PostgresDatabase) {
    setError(null);
    if (!window.confirm(`Drop PostgreSQL database ${database.databaseName}? This cannot be undone.`)) return;
    deleteDatabase.mutate(database.id, {
      onError: (err) => setError(err instanceof Error ? err.message : "Failed to delete database"),
    });
  }

  const canSubmit = databaseName.trim().length > 0 && !createDatabase.isPending;

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <Database className="size-4 text-muted-foreground" />
          Managed Databases
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        <FeatureNote title="What this does:">
          A database is where your application stores its tables and data. Pick an owner role so the app can connect with its own username and password.
        </FeatureNote>
        <p className="text-xs text-muted-foreground">
          Create application databases and optionally attach ownership to a managed read/write or admin role.
        </p>

        <div className="rounded-lg border border-border p-4 bg-muted/10">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-[1fr_12rem_auto] md:items-end">
            <div className="space-y-1.5">
              <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Database Name</label>
              <input
                type="text"
                value={databaseName}
                onChange={(e) => setDatabaseName(e.target.value)}
                placeholder="app_production"
                className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              />
            </div>
            <div className="space-y-1.5">
              <label className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Owner Role (optional)</label>
              <select
                value={ownerRoleId}
                onChange={(e) => setOwnerRoleId(e.target.value)}
                className="w-full px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              >
                <option value="" className="bg-popover text-popover-foreground">None (admin-owned)</option>
                {roles.map((r) => (
                  <option key={r.id} value={r.id} className="bg-popover text-popover-foreground">
                    {r.roleName} ({r.roleKind.replace("_", " ")})
                  </option>
                ))}
              </select>
            </div>
            <Button
              onClick={handleCreate}
              disabled={!canSubmit}
              size="sm"
            >
              {createDatabase.isPending ? "Creating..." : "Create DB"}
            </Button>
          </div>
          {error && <p className="mt-3 text-xs font-semibold text-destructive">{error}</p>}
        </div>

        {isLoading ? (
          <PageSpinner />
        ) : databases.length === 0 ? (
          <div className="py-8 text-center border border-dashed rounded-lg border-border/80">
            <p className="text-xs text-muted-foreground">
              No databases yet.
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/30">
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Database</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Owner</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Status</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Connection URI Template</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4 text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {databases.map((database) => (
                  <TableRow key={database.id} className="hover:bg-muted/10 border-b border-border/40 last:border-none">
                    <TableCell className="px-4 py-2.5 font-semibold text-foreground">{database.databaseName}</TableCell>
                    <TableCell className="px-4 py-2.5 text-foreground/80 text-xs">{database.ownerRoleName || "Skylex admin"}</TableCell>
                    <TableCell className="px-4 py-2.5"><RoleStatusBadge status={database.status} /></TableCell>
                    <TableCell className="px-4 py-2.5">
                      {host ? (
                        <div className="space-y-1">
                          <div className="flex items-center gap-1.5">
                            <code className="max-w-[28rem] truncate text-xs text-muted-foreground font-mono bg-muted/40 px-1.5 py-0.5 rounded border border-border/40">
                              {databaseConnectionURI(
                                host,
                                port,
                                database.databaseName,
                                sslMode,
                                database.ownerRoleName,
                                revealedDatabasePassword(database),
                              )}
                            </code>
                            <CopyButton text={databaseConnectionURI(
                              host,
                              port,
                              database.databaseName,
                              sslMode,
                              database.ownerRoleName,
                              revealedDatabasePassword(database),
                            )} />
                          </div>
                          <div className="flex items-center gap-1.5">
                            <code className="max-w-[28rem] truncate text-xs text-muted-foreground/60 font-mono bg-muted/20 px-1.5 py-0.5 rounded border border-border/20">
                              {databasePsqlCommand(host, port, database.databaseName, sslMode, database.ownerRoleName)}
                            </code>
                            <CopyButton text={databasePsqlCommand(host, port, database.databaseName, sslMode, database.ownerRoleName)} />
                          </div>
                        </div>
                      ) : (
                        <span className="text-xs text-muted-foreground">Endpoint unavailable</span>
                      )}
                    </TableCell>
                    <TableCell className="px-4 py-2.5 text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDelete(database)}
                        disabled={database.status === "deleting" || deleteDatabase.isPending}
                        className="text-destructive hover:text-destructive hover:bg-destructive/10 text-xs font-medium h-7 px-2"
                      >
                        Delete
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
