import { Stack } from "expo-router";
import type { ReactElement } from "react";
import { ActivityIndicator, Alert, FlatList, StyleSheet, Text, View } from "react-native";
import type { Track } from "@/api/types";
import { addToEnd, playFrom, playNext } from "@/player";
import { colors, font } from "@/theme";
import { Btn } from "./player-ui";
import { Cover, Message, TrackRow } from "./ui";

// TrackListDetail is the body of the artist, release, playlist and show
// screens: a header, optional extra sections, then the tracks. Tapping a
// track plays it with the rest of the list queued after it.
export function TrackListDetail({
  title,
  subtitle,
  imageUrl,
  tracks,
  loading,
  error,
  extra,
  emptyText = "No tracks.",
  queueAll,
}: {
  title?: string;
  subtitle?: string;
  imageUrl?: string;
  tracks?: Track[];
  loading: boolean;
  error: Error | null;
  extra?: ReactElement | null;
  emptyText?: string;
  // a Queue button in the header: Play next / Add to queue for the whole list
  queueAll?: boolean;
}) {
  if (loading) return <ActivityIndicator style={styles.fill} color={colors.accent} />;
  if (error) return <Message error>{error.message}</Message>;
  const list = tracks ?? [];
  return (
    <>
      <Stack.Screen options={{ title: title ?? "" }} />
      <FlatList
        style={{ backgroundColor: colors.bg }}
        data={list}
        keyExtractor={(t) => t.id}
        renderItem={({ item, index }) => <TrackRow track={item} onPress={() => playFrom(list, index)} />}
        ListHeaderComponent={
          <View>
            <View style={styles.header}>
              {imageUrl ? <Cover uri={imageUrl} size={96} /> : null}
              <View style={{ flex: 1 }}>
                <Text style={styles.title}>{title}</Text>
                {subtitle ? <Text style={styles.sub}>{subtitle}</Text> : null}
              </View>
            </View>
            {queueAll && list.some((t) => t.streamUrl) ? (
              <View style={styles.actions}>
                <Btn
                  label="Queue…"
                  onPress={() =>
                    Alert.alert(title ?? "Queue", undefined, [
                      { text: "Play next", onPress: () => playNext(list) },
                      { text: "Add to end of queue", onPress: () => addToEnd(list) },
                      { text: "Cancel", style: "cancel" },
                    ])
                  }
                />
              </View>
            ) : null}
            {extra}
          </View>
        }
        ListEmptyComponent={<Message>{emptyText}</Message>}
      />
    </>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1, backgroundColor: colors.bg },
  header: { flexDirection: "row", gap: 16, padding: 16, alignItems: "center" },
  title: { color: colors.fg, fontSize: font.xl, fontWeight: "700" },
  actions: { flexDirection: "row", paddingHorizontal: 8, paddingBottom: 8 },
  sub: { color: colors.fgMuted, fontSize: font.base, marginTop: 4 },
});
