import { useRouter } from "expo-router";
import { FlatList, Pressable, StyleSheet, Text, View } from "react-native";
import { Btn, RepeatButton, SleepTimer, Transport } from "@/components/player-ui";
import { artistNames, Cover, Message } from "@/components/ui";
import { jumpTo, move, removeAt, usePlayer } from "@/player";
import { current } from "@/player/queue";
import { isEpisode } from "@/player/positions";
import { colors, font } from "@/theme";

// Full-screen player: cover, title, seek bar, controls and the queue.
export default function PlayerScreen() {
  const router = useRouter();
  const p = usePlayer();
  const t = current(p.queue);
  if (!t) {
    return (
      <View style={{ flex: 1, backgroundColor: colors.bg }}>
        <Message>Nothing is playing.</Message>
        <Btn label="Close" onPress={() => router.back()} />
      </View>
    );
  }
  const { items, index } = p.queue;
  return (
    <FlatList
      style={{ backgroundColor: colors.bg }}
      data={items}
      keyExtractor={(x) => x.id}
      ListHeaderComponent={
        <View style={styles.head}>
          <View style={{ alignItems: "center", marginBottom: 16 }}>
            <Cover uri={t.imageUrl} size={240} />
          </View>
          <Text style={styles.title}>{t.title}</Text>
          <Text style={styles.sub}>{artistNames(t)}</Text>
          <View style={{ height: 16 }} />
          <Transport />
          <RepeatButton />
          <SleepTimer episode={isEpisode(t)} />
          <Text style={styles.section}>Queue</Text>
        </View>
      }
      renderItem={({ item, index: i }) => {
        const isCurrent = i === index;
        return (
          <View style={[styles.qrow, isCurrent && { backgroundColor: colors.accentDim }]}>
            <Pressable style={{ flex: 1 }} onPress={() => jumpTo(i)}>
              <Text style={[styles.qtitle, isCurrent && { color: colors.accent }]} numberOfLines={1}>
                {item.title}
              </Text>
              <Text style={styles.qsub} numberOfLines={1}>
                {artistNames(item)}
              </Text>
            </Pressable>
            {!isCurrent ? (
              <>
                <Btn label="Up" onPress={() => move(i, i - 1)} disabled={i === 0 || i - 1 === index} />
                <Btn label="Down" onPress={() => move(i, i + 1)} disabled={i === items.length - 1 || i + 1 === index} />
              </>
            ) : null}
            <Btn label="Remove" onPress={() => removeAt(i)} />
          </View>
        );
      }}
    />
  );
}

const styles = StyleSheet.create({
  head: { padding: 16 },
  title: { color: colors.fg, fontSize: font.xl, fontWeight: "700", textAlign: "center" },
  sub: { color: colors.fgMuted, fontSize: font.lg, textAlign: "center", marginTop: 4 },
  section: { color: colors.fgMuted, fontSize: font.sm, fontWeight: "600", textTransform: "uppercase", marginTop: 28 },
  qrow: { flexDirection: "row", alignItems: "center", paddingHorizontal: 12, paddingVertical: 6, borderBottomColor: colors.line, borderBottomWidth: StyleSheet.hairlineWidth },
  qtitle: { color: colors.fg, fontSize: font.base },
  qsub: { color: colors.fgMuted, fontSize: font.sm },
});
