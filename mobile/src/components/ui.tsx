import type { ReactElement, ReactNode } from "react";
import { ActivityIndicator, Alert, FlatList, Image, Pressable, RefreshControl, StyleSheet, Text, View } from "react-native";
import { absoluteUrl, authHeaders } from "@/api/client";
import type { Track } from "@/api/types";
import { addToEnd, playNext } from "@/player";
import { colors, font, radius } from "@/theme";

export function formatDuration(seconds: number): string {
  const s = Math.max(0, Math.round(seconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(sec)}` : `${m}:${pad(sec)}`;
}

export function artistNames(t: Pick<Track, "artists">): string {
  return t.artists.map((a) => a.name).join(", ");
}

// Cover shows an image from the server with the auth header, or a plain tile.
export function Cover({ uri, size = 44 }: { uri?: string; size?: number }) {
  const box = { width: size, height: size, borderRadius: radius.md, backgroundColor: colors.panel2 };
  if (!uri) return <View style={box} />;
  return <Image style={box} source={{ uri: absoluteUrl(uri), headers: authHeaders() }} />;
}

export function Row({ title, subtitle, right, image, onPress, onLongPress }: { title: string; subtitle?: string; right?: string; image?: ReactNode; onPress?: () => void; onLongPress?: () => void }) {
  return (
    <Pressable style={styles.row} onPress={onPress} onLongPress={onLongPress}>
      {image}
      <View style={styles.rowText}>
        <Text style={styles.rowTitle} numberOfLines={1}>
          {title}
        </Text>
        {subtitle ? (
          <Text style={styles.rowSub} numberOfLines={1}>
            {subtitle}
          </Text>
        ) : null}
      </View>
      {right ? <Text style={styles.rowRight}>{right}</Text> : null}
    </Pressable>
  );
}

export function TrackRow({ track, onPress }: { track: Track; onPress: () => void }) {
  const done = track.finished;
  return (
    <Row
      image={<Cover uri={track.imageUrl} />}
      title={track.title}
      subtitle={artistNames(track)}
      right={(done ? "played  " : "") + formatDuration(track.duration)}
      onPress={onPress}
      onLongPress={() =>
        Alert.alert(track.title, undefined, [
          { text: "Play next", onPress: () => playNext(track) },
          { text: "Add to end of queue", onPress: () => addToEnd(track) },
          { text: "Cancel", style: "cancel" },
        ])
      }
    />
  );
}

export function SectionTitle({ children }: { children: string }) {
  return <Text style={styles.section}>{children}</Text>;
}

export function Message({ children, error }: { children: string; error?: boolean }) {
  return (
    <View style={styles.center}>
      <Text style={[styles.message, error && { color: colors.failed }]}>{children}</Text>
    </View>
  );
}

type ListSource<T> = {
  items: T[];
  isLoading: boolean;
  isRefetching: boolean;
  error: Error | null;
  refetch: () => unknown;
  fetchNextPage: () => void;
  isFetchingNextPage: boolean;
};

// PagedList renders a paged hook result as a FlatList with pull to refresh,
// loading more at the end, and loading, error and empty states.
export function PagedList<T>({
  source,
  keyOf,
  render,
  empty,
  header,
}: {
  source: ListSource<T>;
  keyOf: (item: T) => string;
  render: (item: T, index: number) => ReactElement;
  empty: string;
  header?: ReactElement;
}) {
  if (source.isLoading) return <ActivityIndicator style={styles.center} color={colors.accent} />;
  if (source.error) return <Message error>{source.error.message}</Message>;
  return (
    <FlatList
      style={{ backgroundColor: colors.bg }}
      data={source.items}
      keyExtractor={keyOf}
      renderItem={({ item, index }) => render(item, index)}
      ListHeaderComponent={header}
      ListEmptyComponent={<Message>{empty}</Message>}
      ListFooterComponent={source.isFetchingNextPage ? <ActivityIndicator color={colors.accent} style={{ margin: 16 }} /> : undefined}
      onEndReached={source.fetchNextPage}
      onEndReachedThreshold={0.5}
      refreshControl={<RefreshControl refreshing={source.isRefetching} onRefresh={() => void source.refetch()} tintColor={colors.accent} />}
    />
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", alignItems: "center", gap: 12, paddingHorizontal: 16, paddingVertical: 8, borderBottomColor: colors.line, borderBottomWidth: StyleSheet.hairlineWidth },
  rowText: { flex: 1 },
  rowTitle: { color: colors.fg, fontSize: font.lg },
  rowSub: { color: colors.fgMuted, fontSize: font.sm, marginTop: 2 },
  rowRight: { color: colors.fgSubtle, fontSize: font.sm },
  section: { color: colors.fgMuted, fontSize: font.sm, fontWeight: "600", textTransform: "uppercase", letterSpacing: 0.5, paddingHorizontal: 16, paddingTop: 20, paddingBottom: 6 },
  center: { flex: 1, alignItems: "center", justifyContent: "center", padding: 24, backgroundColor: colors.bg },
  message: { color: colors.fgMuted, fontSize: font.base, textAlign: "center" },
});
