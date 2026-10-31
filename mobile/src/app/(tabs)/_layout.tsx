import { Ionicons } from "@expo/vector-icons";
import { BottomTabBar, Tabs } from "expo-router/js-tabs";
import { useRouter } from "expo-router";
import { ColorValue, Pressable, View } from "react-native";
import { MiniPlayer } from "@/components/player-ui";
import { useMode } from "@/modeProvider";
import { colors } from "@/theme";

type IconName = keyof typeof Ionicons.glyphMap;

// tabIcon is a tab's icon: filled when focused, the outline otherwise.
function tabIcon(name: string) {
  return ({ color, size, focused }: { color: ColorValue; size: number; focused: boolean }) => (
    <Ionicons name={(focused ? name : `${name}-outline`) as IconName} size={size} color={color as string} />
  );
}

export default function TabsLayout() {
  const { secret } = useMode();
  const router = useRouter();
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
        headerRight: () => (
          <Pressable
            onPress={() => router.push("/settings")}
            hitSlop={8}
            style={{ paddingHorizontal: 16 }}
            accessibilityRole="button"
            accessibilityLabel={secret ? "Vault" : "Settings"}
          >
            <Ionicons
              name={secret ? "lock-open" : "settings-outline"}
              size={22}
              color={secret ? colors.accent : colors.fgMuted}
            />
          </Pressable>
        ),
        tabBarStyle: { backgroundColor: colors.panel, borderTopColor: colors.line },
        tabBarActiveTintColor: colors.accent,
        tabBarInactiveTintColor: colors.fgMuted,
        tabBarLabelStyle: { fontSize: 12 },
        sceneStyle: { backgroundColor: colors.bg },
      }}
    >
      <Tabs.Screen name="index" options={{ title: "Tracks", tabBarIcon: tabIcon("musical-notes") }} />
      <Tabs.Screen name="artists" options={{ title: "Artists", tabBarIcon: tabIcon("person") }} />
      <Tabs.Screen name="releases" options={{ title: "Releases", tabBarIcon: tabIcon("disc"), href: secret ? null : undefined }} />
      <Tabs.Screen name="playlists" options={{ title: "Playlists", tabBarIcon: tabIcon("list") }} />
      <Tabs.Screen name="podcasts" options={{ title: "Podcasts", tabBarIcon: tabIcon("mic") }} />
      <Tabs.Screen name="search" options={{ title: "Search", tabBarIcon: tabIcon("search") }} />
    </Tabs>
  );
}
