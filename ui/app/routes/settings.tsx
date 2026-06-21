import { useState } from "react";
import { useUsers } from "~/hooks/useUsers";
import { Badge } from "~/components/Badge";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import { Modal } from "~/components/Modal";
import { PageSpinner } from "~/components/Spinner";
import { api } from "~/lib/api";
import { Button } from "~/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "~/components/ui/table";
import { Settings, PlusIcon, Trash2 } from "lucide-react";

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
    <div className="space-y-8 animate-in fade-in duration-300">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-foreground">Settings</h2>
          <p className="text-xs text-muted-foreground mt-1">Configure global control plane settings, users, and roles.</p>
        </div>
        <Button onClick={() => setShowCreate(true)} variant="default" size="sm">
          <PlusIcon className="size-3.5 mr-1.5" />
          Add User
        </Button>
      </div>

      {success && (
        <div className="bg-emerald-50/60 border border-emerald-200/50 text-emerald-700 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50 px-3 py-2.5 rounded-lg text-xs font-medium">
          {success}
        </div>
      )}

      <Card className="shadow-xs">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
            <Settings className="size-4 text-muted-foreground" />
            Control Plane Users ({users.length})
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {users.length === 0 ? (
            <div className="py-16 text-center">
              <p className="text-sm text-muted-foreground">No users found.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/30">
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Email</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Name</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Role</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Created</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {users.map((u) => (
                    <TableRow key={u.id} className="hover:bg-muted/30 transition-colors">
                      <TableCell className="px-6 py-3.5 text-foreground font-semibold">{u.email}</TableCell>
                      <TableCell className="px-6 py-3.5 text-foreground">{u.displayName || "-"}</TableCell>
                      <TableCell className="px-6 py-3.5"><Badge label={u.role} /></TableCell>
                      <TableCell className="text-muted-foreground px-6 py-3.5 text-xs">
                        {new Date(u.createdAt).toLocaleString()}
                      </TableCell>
                      <TableCell className="px-6 py-3.5 text-right">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleDeleteUser(u.id)}
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

      <Modal open={showCreate} title="Create User" onClose={() => setShowCreate(false)}>
        <form onSubmit={handleCreateUser} className="space-y-4">
          {error && (
            <div className="bg-destructive/10 border border-destructive/20 text-destructive px-3 py-2.5 rounded-lg text-xs font-medium">
              {error}
            </div>
          )}
          
          <div className="space-y-1.5">
            <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Email Address</label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              placeholder="e.g. dev@skylex.local"
            />
          </div>

          <div className="space-y-1.5">
            <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Display Name</label>
            <input
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              placeholder="e.g. Developer Name"
            />
          </div>

          <div className="space-y-1.5">
            <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Password</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
            />
          </div>

          <div className="space-y-1.5">
            <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Role</label>
            <select
              value={role}
              onChange={(e) => setRole(e.target.value)}
              className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
            >
              <option value="ADMIN" className="bg-popover text-popover-foreground">Admin</option>
              <option value="OPERATOR" className="bg-popover text-popover-foreground">Operator</option>
              <option value="VIEWER" className="bg-popover text-popover-foreground">Viewer</option>
            </select>
          </div>

          <div className="flex gap-3 pt-4 border-t border-border mt-6">
            <Button type="submit" size="sm">
              Create User
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={() => setShowCreate(false)}>
              Cancel
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}