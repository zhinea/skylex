import { useEffect } from "react";
import { NavLink, Outlet, useNavigate } from "react-router";
import { useAuth } from "~/lib/auth";

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
  const { user, isAuthenticated, logout } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    if (!isAuthenticated) {
      navigate("/login", { replace: true });
    }
  }, [isAuthenticated, navigate]);

  if (!isAuthenticated) {
    return null;
  }

  return (
    <div className="flex h-screen bg-gray-50 dark:bg-gray-900">
      <aside className="w-64 bg-white dark:bg-gray-800 border-r border-gray-200 dark:border-gray-700 flex flex-col">
        <div className="h-16 flex items-center px-6 border-b border-gray-200 dark:border-gray-700">
          <h1 className="text-xl font-bold text-gray-900 dark:text-white">Skylex</h1>
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
                    ? "bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-400"
                    : "text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                }`
              }
            >
              <span className="w-5 text-center">{item.icon}</span>
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="p-4 border-t border-gray-200 dark:border-gray-700">
          {user && (
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-gray-500 dark:text-gray-400 truncate max-w-[140px]">
                {user.email}
              </span>
              <button
                onClick={logout}
                className="text-xs text-red-600 hover:text-red-800 dark:text-red-400 dark:hover:text-red-300"
              >
                Logout
              </button>
            </div>
          )}
          <div className="text-xs text-gray-500 dark:text-gray-400">Skylex v0.1.0</div>
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