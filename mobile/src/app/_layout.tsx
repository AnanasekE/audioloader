import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Redirect, Stack, useSegments } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { useEffect, useState, type ReactNode } from "react";
import { configure } from "@/api/client";
import { ModeProvider, useMode } from "@/modeProvider";
import { bindMode } from "@/player";
import { ServerProvider, useServer } from "@/serverContext";
import { colors } from "@/theme";

// PlayerBoot tells the player which mode we are in (and restores the saved
// queue of that mode on the first call).
function PlayerBoot({ children }: { children: ReactNode }) {
  const { loading, secret } = useMode();
  useEffect(() => {
    if (!loading) void bindMode(secret);
  }, [loading, secret]);
  return <>{children}</>;
}

function Gate() {
  const { loading, url } = useServer();
  const segments = useSegments();
  if (loading) return null;
  const onSetup = segments[0] === "setup";
  if (!url && !onSetup) return <Redirect href="/setup" />;
  // set during render so the first queries of the screens below use it
  if (url) configure(url);
  const stack = (
    <Stack screenOptions={{ headerStyle: { backgroundColor: colors.panel }, headerTintColor: colors.fg, contentStyle: { backgroundColor: colors.bg } }}>
      <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
      <Stack.Screen name="player" options={{ title: "Now playing", presentation: "modal" }} />
      <Stack.Screen name="setup" options={{ title: "Connect", headerShown: false }} />
    </Stack>
  );
  return url ? <ModeProvider>
      <PlayerBoot>{stack}</PlayerBoot>
    </ModeProvider> : stack;
}

export default function RootLayout() {
  const [client] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: 1, staleTime: 30_000 } } }));
  return (
    <QueryClientProvider client={client}>
      <ServerProvider>
        <StatusBar style="light" />
        <Gate />
      </ServerProvider>
    </QueryClientProvider>
  );
}
