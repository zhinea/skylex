import { useState } from "react";
import { useStorageConfigs, useCreateStorageConfig, useDeleteStorageConfig } from "~/hooks/useStorage";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import { Modal } from "~/components/Modal";
import { PageSpinner } from "~/components/Spinner";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { Button } from "~/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "~/components/ui/table";
import { HardDrive, PlusIcon, Trash2 } from "lucide-react";

export default function StoragePage() {
  const { data, isLoading } = useStorageConfigs();
  const createConfig = useCreateStorageConfig();
  const deleteConfig = useDeleteStorageConfig();
  const [showCreate, setShowCreate] = useState(false);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [bucket, setBucket] = useState("");
  const [region, setRegion] = useState("");
  const [accessKey, setAccessKey] = useState("");
  const [secretKey, setSecretKey] = useState("");
  const [useTls, setUseTls] = useState(false);
  const [error, setError] = useState("");

  if (isLoading) return <PageSpinner />;

  const configs = data?.storageConfigs || [];

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      await createConfig.mutateAsync({ name, endpoint, bucket, region, accessKey, secretKey, useTls });
      setShowCreate(false);
      setName(""); setEndpoint(""); setBucket(""); setRegion(""); setAccessKey(""); setSecretKey("");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to create");
    }
  };

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-foreground">Storage</h2>
          <p className="text-xs text-muted-foreground mt-1">Configure S3-compatible object storage profiles for database backups.</p>
        </div>
        <Button onClick={() => setShowCreate(true)} variant="default" size="sm">
          <PlusIcon className="size-3.5 mr-1.5" />
          Add Storage
        </Button>
      </div>

      <Card className="shadow-xs">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
            <HardDrive className="size-4 text-muted-foreground" />
            Storage Configurations ({configs.length})
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {configs.length === 0 ? (
            <div className="py-16 text-center">
              <p className="text-sm text-muted-foreground">
                No storage configurations. Add an S3-compatible storage backend to enable backups.
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/30">
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Name</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Endpoint</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Bucket</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Region</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">TLS</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {configs.map((c) => (
                    <TableRow key={c.id} className="hover:bg-muted/30 transition-colors">
                      <TableCell className="px-6 py-3.5 text-foreground font-semibold">{c.name}</TableCell>
                      <TableCell className="text-muted-foreground px-6 py-3.5 text-xs font-mono">{c.endpoint}</TableCell>
                      <TableCell className="text-foreground px-6 py-3.5 text-xs">{c.bucket}</TableCell>
                      <TableCell className="text-foreground px-6 py-3.5 text-xs">{c.region || "-"}</TableCell>
                      <TableCell className="text-foreground px-6 py-3.5 text-xs font-semibold">{c.useTls ? "Yes" : "No"}</TableCell>
                      <TableCell className="px-6 py-3.5 text-right">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setDeleteId(c.id)}
                          className="text-destructive hover:text-destructive hover:bg-destructive/10 text-xs font-medium h-7 px-2"
                        >
                          <Trash2 className="size-3 mr-1" />
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

      <Modal open={showCreate} title="Add Storage Config" onClose={() => setShowCreate(false)}>
        <form onSubmit={handleCreate} className="space-y-4">
          {error && (
            <div className="bg-destructive/10 border border-destructive/20 text-destructive px-3 py-2.5 rounded-lg text-xs font-medium">
              {error}
            </div>
          )}
          
          <div className="space-y-1.5">
            <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Configuration Name</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              placeholder="e.g. minio-backup"
            />
          </div>

          <div className="space-y-1.5">
            <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">S3 Endpoint</label>
            <input
              type="text"
              value={endpoint}
              onChange={(e) => setEndpoint(e.target.value)}
              required
              placeholder="e.g. s3.amazonaws.com"
              className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Bucket</label>
              <input
                type="text"
                value={bucket}
                onChange={(e) => setBucket(e.target.value)}
                required
                className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              />
            </div>
            <div className="space-y-1.5">
              <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Region</label>
              <input
                type="text"
                value={region}
                onChange={(e) => setRegion(e.target.value)}
                placeholder="us-east-1"
                className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Access Key</label>
              <input
                type="text"
                value={accessKey}
                onChange={(e) => setAccessKey(e.target.value)}
                required
                className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              />
            </div>
            <div className="space-y-1.5">
              <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Secret Key</label>
              <input
                type="password"
                value={secretKey}
                onChange={(e) => setSecretKey(e.target.value)}
                required
                className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              />
            </div>
          </div>

          <div className="flex items-center gap-2 pt-1">
            <input
              type="checkbox"
              id="useTls"
              checked={useTls}
              onChange={(e) => setUseTls(e.target.checked)}
              className="rounded border-input text-primary focus:ring-ring"
            />
            <label htmlFor="useTls" className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Use Secure TLS Connection
            </label>
          </div>

          <div className="flex gap-3 pt-4 border-t border-border mt-6">
            <Button type="submit" disabled={createConfig.isPending} size="sm">
              {createConfig.isPending ? "Saving..." : "Save Config"}
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={() => setShowCreate(false)}>
              Cancel
            </Button>
          </div>
        </form>
      </Modal>

      <ConfirmDialog
        open={!!deleteId}
        title="Delete Storage Config"
        message="Are you sure? This will remove the storage configuration."
        confirmLabel="Delete"
        onConfirm={() => { if (deleteId) { deleteConfig.mutate(deleteId); setDeleteId(null); }}}
        onCancel={() => setDeleteId(null)}
      />
    </div>
  );
}