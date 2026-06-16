import { useState } from "react";
import { useUsers } from "~/hooks/useUsers";
import { Badge } from "~/components/Badge";
import { Card } from "~/components/Card";
import { Modal } from "~/components/Modal";
import { PageSpinner } from "~/components/Spinner";
import { api } from "~/lib/api";

export default function SettingsPage() {
  const { data, isLoading } = useUsers();
  const [showCreate, setShowCreate] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [role, setRole] = useState("ADMIN");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  if (isLoading) return <PageSpinner />;

  const users = data?.users || [];

  const handleCreateUser = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSuccess("");
    try {
      await api.post("/skylex.v1.AuthService/CreateUser", { email, password, displayName, role });
      setShowCreate(false);
      setEmail(""); setPassword(""); setDisplayName("");
      setSuccess("User created successfully");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to create user");
    }
  };

  const handleDeleteUser = async (userId: string) => {
    try {
      await api.post("/skylex.v1.AuthService/DeleteUser", { id: userId });
      setSuccess("User deleted");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to delete user");
    }
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-white">Settings</h2>
        <button
          onClick={() => setShowCreate(true)}
          className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium rounded-lg"
        >
          Add User
        </button>
      </div>

      {success && (
        <div className="mb-4 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 text-green-700 dark:text-green-400 px-4 py-3 rounded-lg text-sm">
          {success}
        </div>
      )}

      <Card title="Users">
        {users.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400 py-4 text-center">No users found.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Email</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Name</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Role</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Created</th>
                  <th className="text-right px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Actions</th>
                </tr>
              </thead>
              <tbody>
                {users.map((u) => (
                  <tr key={u.id} className="border-b border-gray-100 dark:border-gray-800">
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{u.email}</td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{u.displayName || "-"}</td>
                    <td className="px-4 py-3"><Badge label={u.role} /></td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 text-xs">{new Date(u.createdAt).toLocaleString()}</td>
                    <td className="px-4 py-3 text-right">
                      <button
                        onClick={() => handleDeleteUser(u.id)}
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

      <Modal open={showCreate} title="Create User" onClose={() => setShowCreate(false)}>
        <form onSubmit={handleCreateUser} className="space-y-4">
          {error && (
            <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 px-4 py-3 rounded-lg text-sm">
              {error}
            </div>
          )}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Email</label>
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Display Name</label>
            <input type="text" value={displayName} onChange={(e) => setDisplayName(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Password</label>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Role</label>
            <select value={role} onChange={(e) => setRole(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white">
              <option value="ADMIN">Admin</option>
              <option value="OPERATOR">Operator</option>
              <option value="VIEWER">Viewer</option>
            </select>
          </div>
          <div className="flex gap-3 pt-2">
            <button type="submit"
              className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium rounded-lg">
              Create User
            </button>
            <button type="button" onClick={() => setShowCreate(false)}
              className="px-4 py-2 bg-gray-100 hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 text-sm font-medium rounded-lg">
              Cancel
            </button>
          </div>
        </form>
      </Modal>
    </div>
  );
}