import { useEffect } from "react";
import { NavLink, Outlet, useNavigate } from "react-router";
import { useAuth } from "~/lib/auth";
import { Button } from "~/components/ui/button";
import {
  LayoutDashboard,
  Server,
  Network,
  History,
  HardDrive,
  Settings,
  FileText,
  LogOut,
} from "lucide-react";

const navItems = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard },
  { to: "/clusters", label: "Clusters", icon: Server },
  { to: "/nodes", label: "Nodes", icon: Network },
  { to: "/backups", label: "Backups", icon: History },
  { to: "/storage", label: "Storage", icon: HardDrive },
  { to: "/settings", label: "Settings", icon: Settings },
  { to: "/audit", label: "Audit Logs", icon: FileText },
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
      <aside className="w-60 bg-sidebar text-sidebar-foreground border-r border-sidebar-border flex flex-col">
        <div className="h-14 flex items-center px-6 border-b border-sidebar-border">
          <div className="flex items-center gap-2">
            <span className="text-lg font-bold tracking-tight text-foreground">Skylex</span>
            <span className="text-[10px] font-medium px-1.5 py-0.5 rounded bg-muted text-muted-foreground border">Beta</span>
          </div>
        </div>
        <nav className="flex-1 px-3 py-4 space-y-1">
          {navItems.map((item) => {
            const Icon = item.icon;
            return (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === "/"}
                className={({ isActive }) =>
                  `flex items-center gap-2.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${
                    isActive
                      ? "bg-sidebar-accent text-sidebar-accent-foreground font-semibold"
                      : "text-sidebar-foreground/75 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground"
                  }`
                }
              >
                <Icon className="size-4 shrink-0" />
                {item.label}
              </NavLink>
            );
          })}
        </nav>
        <div className="p-4 border-t border-sidebar-border space-y-2 bg-muted/20">
          {user && (
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs text-sidebar-foreground/60 truncate" title={user.email}>
                {user.email}
              </span>
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={logout}
                title="Logout"
                className="text-muted-foreground hover:text-destructive hover:bg-destructive/10 shrink-0"
              >
                <LogOut className="size-3.5" />
              </Button>
            </div>
          )}
          <div className="text-[10px] text-sidebar-foreground/40">Skylex v0.1.0</div>
        </div>
      </aside>
      <main className="flex-1 overflow-auto bg-background">
        <div className="p-8 max-w-7xl mx-auto">
          <Outlet />
        </div>
      </main>
    </div>
  );
}

