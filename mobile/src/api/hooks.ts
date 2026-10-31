import { useQuery } from "@tanstack/react-query";
import { get } from "./client";
import type { Me } from "./types";

// One hook per endpoint. Query keys start with a name; "me" is the only key
// that survives a mode change.
export const meKey = ["me"] as const;

export function useMe() {
  return useQuery({
    queryKey: meKey,
    queryFn: () => get<Me>("/api/v1/me"),
    refetchInterval: 60_000,
  });
}
