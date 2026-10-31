import { Redirect, Stack, useSegments } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { ServerProvider, useServer } from "@/serverContext";
import { colors } from "@/theme";

function Gate() {
  const { loading, url } = useServer();
  const segments = useSegments();
  if (loading) return null;
  const onSetup = segments[0] === "setup";
  if (!url && !onSetup) return <Redirect href="/setup" />;
  return (
    <Stack screenOptions={{ headerStyle: { backgroundColor: colors.panel }, headerTintColor: colors.fg, contentStyle: { backgroundColor: colors.bg } }}>
      <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
      <Stack.Screen name="setup" options={{ title: "Connect", headerShown: false }} />
    </Stack>
  );
}

export default function RootLayout() {
  return (
    <ServerProvider>
      <StatusBar style="light" />
      <Gate />
    </ServerProvider>
  );
}
