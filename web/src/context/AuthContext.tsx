import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  type ReactNode,
} from "react";

type AuthRole = "operator" | "tenant";

interface AuthState {
  serverUrl: string;
  token: string;
  isAuthenticated: boolean;
  role: AuthRole;
  tenantName?: string;
  tier?: string;
}

interface AuthContextValue extends AuthState {
  login: (token: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

const STORAGE_KEY = "codeforge_auth";

function loadAuth(): AuthState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as {
        serverUrl: string;
        token: string;
        role?: AuthRole;
        tenantName?: string;
        tier?: string;
      };
      if (parsed.token) {
        return {
          serverUrl: parsed.serverUrl,
          token: parsed.token,
          isAuthenticated: true,
          // Legacy entries have no role — treat them as operator.
          role: parsed.role === "tenant" ? "tenant" : "operator",
          tenantName: parsed.tenantName,
          tier: parsed.tier,
        };
      }
    }
  } catch {
    // ignore
  }
  return { serverUrl: "", token: "", isAuthenticated: false, role: "operator" };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(loadAuth);

  useEffect(() => {
    if (state.isAuthenticated) {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          serverUrl: state.serverUrl,
          token: state.token,
          role: state.role,
          tenantName: state.tenantName,
          tier: state.tier,
        }),
      );
    } else {
      localStorage.removeItem(STORAGE_KEY);
    }
  }, [state]);

  const login = useCallback(async (token: string) => {
    const res = await fetch("/api/v1/auth/verify", {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!res.ok) {
      if (res.status === 401) throw new Error("Invalid token");
      throw new Error("Server unreachable");
    }
    // Legacy servers return only {"status":"authenticated"} — default to operator.
    const body = (await res.json().catch(() => ({}))) as {
      role?: string;
      tenant_name?: string;
      tier?: string;
    };
    const role: AuthRole = body.role === "tenant" ? "tenant" : "operator";
    setState({
      serverUrl: "",
      token,
      isAuthenticated: true,
      role,
      tenantName: role === "tenant" ? body.tenant_name : undefined,
      tier: role === "tenant" ? body.tier : undefined,
    });
  }, []);

  const logout = useCallback(() => {
    setState({
      serverUrl: "",
      token: "",
      isAuthenticated: false,
      role: "operator",
    });
  }, []);

  return (
    <AuthContext.Provider value={{ ...state, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}

export { AuthContext };
export type { AuthContextValue, AuthState, AuthRole };
