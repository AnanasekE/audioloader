import { useRouter } from "expo-router";
import { useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { next, previous, seekTo, skipBy, toggle, usePlayer } from "@/player";
import { current } from "@/player/queue";
import { colors, font, radius } from "@/theme";
import { artistNames, Cover, formatDuration } from "./ui";

// A tap-to-seek progress bar (no slider dependency).
export function SeekBar({ position, duration, thin }: { position: number; duration: number; thin?: boolean }) {
  const [width, setWidth] = useState(0);
  const frac = duration > 0 ? Math.min(1, position / duration) : 0;
  return (
    <Pressable
      onLayout={(e) => setWidth(e.nativeEvent.layout.width)}
      onPress={(e) => {
        if (!thin && width > 0 && duration > 0) void seekTo((e.nativeEvent.locationX / width) * duration);
      }}
      hitSlop={thin ? undefined : { top: 12, bottom: 12 }}
      style={[styles.bar, thin && { height: 2 }]}
    >
      <View style={[styles.barFill, { width: `${frac * 100}%` }]} />
    </Pressable>
  );
}

export function Btn({ label, onPress, big, disabled }: { label: string; onPress: () => void; big?: boolean; disabled?: boolean }) {
  return (
    <Pressable onPress={onPress} disabled={disabled} hitSlop={8} style={[styles.btn, big && styles.btnBig, disabled && { opacity: 0.35 }]}>
      <Text style={[styles.btnText, big && { color: colors.accentInk, fontSize: font.xl }]}>{label}</Text>
    </Pressable>
  );
}

// MiniPlayer sits above the tab bar while something is queued.
export function MiniPlayer() {
  const router = useRouter();
  const p = usePlayer();
  const t = current(p.queue);
  if (!t) return null;
  return (
    <View style={styles.mini}>
      <SeekBar position={p.position} duration={p.duration || t.duration} thin />
      <Pressable style={styles.miniRow} onPress={() => router.push("/player")}>
        <Cover uri={t.imageUrl} size={40} />
        <View style={{ flex: 1 }}>
          <Text style={styles.miniTitle} numberOfLines={1}>
            {t.title}
          </Text>
          <Text style={styles.miniSub} numberOfLines={1}>
            {artistNames(t)}
          </Text>
        </View>
        <Btn label={p.playing ? "Pause" : "Play"} onPress={toggle} />
        <Btn label="Next" onPress={next} disabled={p.queue.index + 1 >= p.queue.items.length} />
      </Pressable>
    </View>
  );
}

export function Transport() {
  const p = usePlayer();
  const t = current(p.queue);
  const duration = p.duration || t?.duration || 0;
  return (
    <View>
      <SeekBar position={p.position} duration={duration} />
      <View style={styles.times}>
        <Text style={styles.time}>{formatDuration(p.position)}</Text>
        <Text style={styles.time}>{formatDuration(duration)}</Text>
      </View>
      <View style={styles.controls}>
        <Btn label="Prev" onPress={previous} />
        <Btn label="-15" onPress={() => void skipBy(-15)} />
        <Btn label={p.playing ? "Pause" : "Play"} onPress={toggle} big />
        <Btn label="+30" onPress={() => void skipBy(30)} />
        <Btn label="Next" onPress={next} disabled={p.queue.index + 1 >= p.queue.items.length} />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  bar: { height: 4, backgroundColor: colors.line, borderRadius: 2, overflow: "hidden" },
  barFill: { height: "100%", backgroundColor: colors.accent },
  btn: { paddingHorizontal: 10, paddingVertical: 8, borderRadius: radius.md },
  btnBig: { backgroundColor: colors.accent, paddingHorizontal: 22 },
  btnText: { color: colors.fg, fontSize: font.base, fontWeight: "600" },
  mini: { backgroundColor: colors.panel2, borderTopColor: colors.line, borderTopWidth: StyleSheet.hairlineWidth },
  miniRow: { flexDirection: "row", alignItems: "center", gap: 8, paddingHorizontal: 12, paddingVertical: 8 },
  miniTitle: { color: colors.fg, fontSize: font.base, fontWeight: "600" },
  miniSub: { color: colors.fgMuted, fontSize: font.sm },
  times: { flexDirection: "row", justifyContent: "space-between", marginTop: 6 },
  time: { color: colors.fgMuted, fontSize: font.sm },
  controls: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginTop: 16 },
});
