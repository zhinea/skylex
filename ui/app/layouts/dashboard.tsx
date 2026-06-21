import { useEffect } from "react";
import { NavLink, Outlet, useNavigate } from "react-router";
import { useAuth } from "~/lib/auth";
import { Button } from "~/components/ui/button";

const navItems = [
  { to: "/", label: "Dashboard", icon: "□" },
  { to: "/clusters", label: "Clusters", icon: "◈" },
  { to: "/nodes", label: "Nodes", icon: "◉" },
  { to: "/backups", label: "Backups", icon: "↻" },
  { to: "/storage", label: "Storage", icon: "☰" },
  { to: "/settings", label: "Settings", icon: "⚙" },
  { to: "/audit", label: "Audit Logs", icon: "≡" },
];

export default function DashboardLayout() {
  const { user, isAuthenticated, isLoading, logout } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      navigate("/login", { replace: true });
    }
  }, [isLoading, isAuthenticated, navigate]);

  if (isLoading || !isAuthenticated) {
    return null;
  }

  return (
    <div className="flex h-screen bg-background text-foreground">
      <aside className="w-64 bg-sidebar text-sidebar-foreground border-r border-sidebar-border flex flex-col">
        <div className="h-16 flex items-center px-6 border-b border-sidebar-border">
          <h1 className="text-xl font-bold">Skylex</h1>
        </div>
        <nav className="flex-1 px-4 py-4 space-y-1">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === "/"}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                  isActive
                    ? "bg-sidebar-accent text-sidebar-accent-foreground font-semibold"
                    : "text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
                }`
              }
            >
              <span className="w-5 text-center">{item.icon}</span>
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="p-4 border-t border-sidebar-border">
          {user && (
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-sidebar-foreground/50 truncate max-w-[140px]">
                {user.email}
              </span>
              <Button
                variant="ghost"
                size="xs"
                onClick={logout}
                className="text-xs text-destructive hover:text-destructive hover:bg-destructive/10"
              >
                Logout
              </Button>
            </div>
          )}
          <div className="text-xs text-sidebar-foreground/40">Skylex v0.1.0</div>
        </div>
      </aside>
      <main className="flex-1 overflow-auto">
        <div className="p-8">
          <Outlet />
        </div>
      </main>
    </div>
  );
}

