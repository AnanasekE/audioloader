import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { clearServerUrl, loadServerUrl, saveServerUrl } from "./server";

type ServerState = {
  loading: boolean;
  url: string | null;
  setUrl: (url: string) => Promise<void>;
  forget: () => Promise<void>;
};

const Ctx = createContext<ServerState | null>(null);

// ServerProvider holds the server URL entered on first launch.
export function ServerProvider({ children }: { children: ReactNode }) {
  const [loading, setLoading] = useState(true);
  const [url, setUrlState] = useState<string | null>(null);

  useEffect(() => {
    loadServerUrl()
      .then((u) => setUrlState(u || null))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const setUrl = useCallback(async (u: string) => {
    await saveServerUrl(u);
    setUrlState(u);
  }, []);
  const forget = useCallback(async () => {
    await clearServerUrl();
    setUrlState(null);
  }, []);

  const value = useMemo(() => ({ loading, url, setUrl, forget }), [loading, url, setUrl, forget]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useServer(): ServerState {
  const v = useContext(Ctx);
  if (!v) throw new Error("useServer outside ServerProvider");
  return v;
}
