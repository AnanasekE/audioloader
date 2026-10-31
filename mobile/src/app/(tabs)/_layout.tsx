import { BottomTabBar, Tabs } from "expo-router/js-tabs";
import { View } from "react-native";
import { MiniPlayer } from "@/components/player-ui";
import { useMode } from "@/modeProvider";
import { colors } from "@/theme";

export default function TabsLayout() {
  const { secret } = useMode();
  return (
    <Tabs
      tabBar={(props) => (
        <View>
          <MiniPlayer />
          <BottomTabBar {...props} />
        </View>
      )}
      screenOptions={{
        headerStyle: { backgroundColor: colors.panel },
        headerTintColor: colors.fg,
        tabBarStyle: { backgroundColor: colors.panel, borderTopColor: colors.line },
        tabBarActiveTintColor: colors.accent,
        tabBarInactiveTintColor: colors.fgMuted,
        tabBarIcon: () => null,
        tabBarIconStyle: { display: "none" },
        tabBarLabelStyle: { fontSize: 12, marginBottom: 12 },
        sceneStyle: { backgroundColor: colors.bg },
      }}
    >
      <Tabs.Screen name="index" options={{ title: "Tracks" }} />
      <Tabs.Screen name="artists" options={{ title: "Artists" }} />
      <Tabs.Screen name="releases" options={{ title: "Releases", href: secret ? null : undefined }} />
      <Tabs.Screen name="playlists" options={{ title: "Playlists" }} />
      <Tabs.Screen name="podcasts" options={{ title: "Podcasts" }} />
      <Tabs.Screen name="search" options={{ title: "Search" }} />
    </Tabs>
  );
}
