import { createContext, useContext, useEffect, useState, useCallback, type ReactNode } from "react";
import { useNavigate } from "react-router";
import { api, setToken, clearToken, ApiError } from "~/lib/api";

interface User {
  id: string;
  email: string;
  displayName: string;
  role: string;
  createdAt: string;
}

interface AuthState {
  user: User | null;
  accessToken: string | null;
  refreshToken: string | null;
  login: (email: string, password: string) => Promise<void>;
  logout: () => void;
  isAuthenticated: boolean;
  isLoading: boolean;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [accessToken, setAccessToken] = useState<string | null>(null);
  const [refreshToken, setRefreshToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const navigate = useNavigate();

  useEffect(() => {
    setAccessToken(localStorage.getItem("skylex_token"));
    setRefreshToken(localStorage.getItem("skylex_refresh_token"));
    const storedUser = localStorage.getItem("skylex_user");
    if (storedUser) {
      try {
        setUser(JSON.parse(storedUser));
      } catch (e) {
        console.error("Failed to parse stored user", e);
      }
    }
    setIsLoading(false);
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    const data = (await api.post("/skylex.v1.AuthService/Login", { email, password })) as Record<string, unknown>;

    const accessTokenVal = (data.accessToken || data.access_token) as string;
    const refreshTokenVal = (data.refreshToken || data.refresh_token) as string;
    const userVal = data.user as User | undefined;

    if (!accessTokenVal || !userVal) {
      throw new Error("Invalid login response from server");
    }

    setToken(accessTokenVal);
    if (refreshTokenVal) {
      localStorage.setItem("skylex_refresh_token", refreshTokenVal);
    }
    localStorage.setItem("skylex_user", JSON.stringify(userVal));
    setAccessToken(accessTokenVal);
    setRefreshToken(refreshTokenVal || null);
    setUser(userVal);
    navigate("/");
  }, [navigate]);

  const logout = useCallback(() => {
    clearToken();
    localStorage.removeItem("skylex_refresh_token");
    localStorage.removeItem("skylex_user");
    setAccessToken(null);
    setRefreshToken(null);
    setUser(null);
    navigate("/login");
  }, [navigate]);

  return (
    <AuthContext.Provider
      value={{
        user,
        accessToken,
        refreshToken,
        login,
        logout,
        isAuthenticated: !!accessToken,
        isLoading,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
