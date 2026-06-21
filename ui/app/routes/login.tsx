import { useState } from "react";
import { useAuth } from "~/lib/auth";
import { Button } from "~/components/ui/button";

export default function LoginPage() {
  const { login } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      await login(email, password);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-background text-foreground relative overflow-hidden">
      {/* Background soft ambient radial gradient */}
      <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-muted/50 via-background to-background pointer-events-none" />
      
      <div className="max-w-sm w-full mx-4 z-10">
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center size-12 rounded-xl bg-primary text-primary-foreground font-bold text-xl shadow-xs mb-4">
            S
          </div>
          <h1 className="text-2xl font-bold tracking-tight text-foreground">Skylex</h1>
          <p className="mt-1 text-xs text-muted-foreground uppercase tracking-wider font-semibold">
            Database Control Plane
          </p>
        </div>

        <div className="bg-card text-card-foreground rounded-xl border border-border p-6 shadow-sm ring-1 ring-foreground/[0.03]">
          <form onSubmit={handleSubmit} className="space-y-4">
            {error && (
              <div className="bg-destructive/10 border border-destructive/20 text-destructive px-3 py-2.5 rounded-lg text-xs font-medium">
                {error}
              </div>
            )}
            
            <div className="space-y-1.5">
              <label htmlFor="email" className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                Email Address
              </label>
              <input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                placeholder="admin@skylex.local"
              />
            </div>

            <div className="space-y-1.5">
              <label htmlFor="password" className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                Password
              </label>
              <input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              />
            </div>

            <Button
              type="submit"
              disabled={loading}
              className="w-full mt-2"
              variant="default"
              size="lg"
            >
              {loading ? "Signing in..." : "Sign In"}
            </Button>
          </form>
        </div>
        
        <div className="mt-8 text-center text-[10px] text-muted-foreground uppercase tracking-widest">
          Skylex Control Plane &bull; v0.1.0
        </div>
      </div>
    </div>
  );
}