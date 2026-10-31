import { focusManager, useQueryClient } from "@tanstack/react-query";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, type ReactNode } from "react";
import { AppState, Platform } from "react-native";
import { meKey, useMe } from "./api/hooks";
import { ModeTracker, resetForModeChange } from "./mode";

type ModeState = {
  loading: boolean;
  error: Error | null;
  secret: boolean;
  vault: boolean;
  secretSource: boolean;
  // reset drops every cached answer, stops playback and empties the queue,
  // then asks /me again. Called after unlock and lock.
  reset: () => Promise<void>;
};

const Ctx = createContext<ModeState | null>(null);

// ModeProvider exposes the mode flags from /api/v1/me and resets caches and
// player whenever the mode changes.
export function ModeProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient();
  const me = useMe();
  const tracker = useRef(new ModeTracker());

  // refetch when the app comes back to the foreground
  useEffect(() => {
    if (Platform.OS === "web") return;
    const sub = AppState.addEventListener("change", (s) => focusManager.setFocused(s === "active"));
    return () => sub.remove();
  }, []);

  const dropAll = useCallback(() => {
    void qc.cancelQueries();
    qc.removeQueries({ predicate: (q) => q.queryKey[0] !== meKey[0] });
    resetForModeChange();
  }, [qc]);

  useEffect(() => {
    if (me.data && tracker.current.update(me.data)) dropAll();
  }, [me.data, dropAll]);

  const reset = useCallback(async () => {
    dropAll();
    await qc.invalidateQueries({ queryKey: meKey });
  }, [dropAll, qc]);

  const value = useMemo<ModeState>(
    () => ({
      loading: me.isLoading,
      error: me.error,
      secret: me.data?.secret ?? false,
      vault: me.data?.vault ?? false,
      secretSource: me.data?.secretSource ?? false,
      reset,
    }),
    [me.isLoading, me.error, me.data, reset],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useMode(): ModeState {
  const v = useContext(Ctx);
  if (!v) throw new Error("useMode outside ModeProvider");
  return v;
}
