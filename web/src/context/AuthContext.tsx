import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  type ReactNode,
} from "react";

interface AuthState {
  serverUrl: string;
  token: string;
  isAuthenticated: boolean;
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
      const parsed = JSON.parse(raw) as { serverUrl: string; token: string };
      if (parsed.token) {
        return {
          serverUrl: parsed.serverUrl,
          token: parsed.token,
          isAuthenticated: true,
        };
      }
    }
  } catch {
    // ignore
  }
  return { serverUrl: "", token: "", isAuthenticated: false };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(loadAuth);

  useEffect(() => {
    if (state.isAuthenticated) {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({ serverUrl: state.serverUrl, token: state.token }),
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
    setState({ serverUrl: "", token, isAuthenticated: true });
  }, []);

  const logout = useCallback(() => {
    setState({ serverUrl: "", token: "", isAuthenticated: false });
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
export type { AuthContextValue, AuthState };
