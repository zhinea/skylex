import { useState } from "react";
import { useStorageConfigs, useCreateStorageConfig, useDeleteStorageConfig } from "~/hooks/useStorage";
import { Card } from "~/components/Card";
import { Modal } from "~/components/Modal";
import { PageSpinner } from "~/components/Spinner";
import { ConfirmDialog } from "~/components/ConfirmDialog";

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
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-white">Storage</h2>
        <button
          onClick={() => setShowCreate(true)}
          className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium rounded-lg"
        >
          Add Storage
        </button>
      </div>

      <Card>
        {configs.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400 py-4 text-center">
            No storage configurations. Add an S3-compatible storage backend to enable backups.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Name</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Endpoint</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Bucket</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Region</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">TLS</th>
                  <th className="text-right px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Actions</th>
                </tr>
              </thead>
              <tbody>
                {configs.map((c) => (
                  <tr key={c.id} className="border-b border-gray-100 dark:border-gray-800">
                    <td className="px-4 py-3 text-gray-900 dark:text-white font-medium">{c.name}</td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 text-xs">{c.endpoint}</td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{c.bucket}</td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{c.region || "-"}</td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{c.useTls ? "Yes" : "No"}</td>
                    <td className="px-4 py-3 text-right">
                      <button
                        onClick={() => setDeleteId(c.id)}
                        className="text-xs text-red-600 hover:text-red-800 dark:text-red-400"
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Modal open={showCreate} title="Add Storage Config" onClose={() => setShowCreate(false)}>
        <form onSubmit={handleCreate} className="space-y-4">
          {error && (
            <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 px-4 py-3 rounded-lg text-sm">
              {error}
            </div>
          )}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Name</label>
            <input type="text" value={name} onChange={(e) => setName(e.target.value)} required
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Endpoint</label>
            <input type="text" value={endpoint} onChange={(e) => setEndpoint(e.target.value)} required placeholder="s3.amazonaws.com"
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Bucket</label>
            <input type="text" value={bucket} onChange={(e) => setBucket(e.target.value)} required
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Region</label>
            <input type="text" value={region} onChange={(e) => setRegion(e.target.value)} placeholder="us-east-1"
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Access Key</label>
            <input type="text" value={accessKey} onChange={(e) => setAccessKey(e.target.value)} required
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Secret Key</label>
            <input type="password" value={secretKey} onChange={(e) => setSecretKey(e.target.value)} required
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
          </div>
          <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input type="checkbox" checked={useTls} onChange={(e) => setUseTls(e.target.checked)}
              className="rounded border-gray-300 dark:border-gray-600" />
            Use TLS
          </label>
          <div className="flex gap-3 pt-2">
            <button type="submit" disabled={createConfig.isPending}
              className="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white text-sm font-medium rounded-lg">
              {createConfig.isPending ? "Saving..." : "Save"}
            </button>
            <button type="button" onClick={() => setShowCreate(false)}
              className="px-4 py-2 bg-gray-100 hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 text-sm font-medium rounded-lg">
              Cancel
            </button>
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