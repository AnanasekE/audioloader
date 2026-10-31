import { useRouter } from "expo-router";
import { useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { cancelSleep, extendSleep, next, previous, seekTo, skipBy, sleepAtEndOfTrack, startSleep, toggle, usePlayer } from "@/player";
import { current } from "@/player/queue";
import * as S from "@/player/sleepTimer";
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
        {S.isActive(p.sleep) ? <Text style={styles.miniSleep}>{S.badge(p.sleep, Date.now())}</Text> : null}
        <Btn label={p.playing ? "Pause" : "Play"} onPress={toggle} />
        <Btn label="Next" onPress={next} disabled={p.queue.index + 1 >= p.queue.items.length} />
      </Pressable>
    </View>
  );
}

// SleepTimer: the first tap with no timer running starts the default 30
// minutes and opens the menu; with one running a tap only opens the menu.
export function SleepTimer({ episode }: { episode: boolean }) {
  const p = usePlayer();
  const [open, setOpen] = useState(false);
  const active = S.isActive(p.sleep);
  const now = Date.now();
  const status = !active ? "Off" : p.sleep.kind === "track" ? `Pauses when this ${episode ? "episode" : "track"} ends` : `Pauses in ${S.minutesLeft(p.sleep, now)} min`;
  return (
    <View style={{ marginTop: 20 }}>
      <Pressable
        style={styles.sleepBtn}
        onPress={() => {
          if (!open && !active) startSleep(S.DEFAULT_MINUTES);
          setOpen(!open);
        }}
      >
        <Text style={[styles.btnText, active && { color: colors.accent }]}>{active ? `Sleep timer ${S.badge(p.sleep, now)}` : "Sleep timer"}</Text>
      </Pressable>
      {open ? (
        <View style={styles.sleepMenu}>
          <Text style={styles.time}>{status}</Text>
          {active ? (
            <View style={styles.chips}>
              {p.sleep.kind === "at" ? <Chip label="+15 min" onPress={() => extendSleep(15)} /> : null}
              <Chip
                label="Off"
                onPress={() => {
                  cancelSleep();
                  setOpen(false);
                }}
              />
            </View>
          ) : null}
          <View style={styles.chips}>
            {S.MINUTES.map((m) => (
              <Chip key={m} label={`${m} min`} onPress={() => startSleep(m)} />
            ))}
            <Chip label={episode ? "End of this episode" : "End of this track"} onPress={sleepAtEndOfTrack} />
          </View>
        </View>
      ) : null}
    </View>
  );
}

function Chip({ label, onPress }: { label: string; onPress: () => void }) {
  return (
    <Pressable onPress={onPress} style={styles.chip}>
      <Text style={styles.btnText}>{label}</Text>
    </Pressable>
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
  miniSleep: { color: colors.accent, fontSize: font.sm, fontWeight: "600" },
  sleepBtn: { alignSelf: "center", paddingHorizontal: 14, paddingVertical: 8, borderRadius: radius.md, backgroundColor: colors.panel2 },
  sleepMenu: { marginTop: 10, gap: 10, padding: 12, borderRadius: radius.lg, backgroundColor: colors.panel, borderColor: colors.line, borderWidth: StyleSheet.hairlineWidth },
  chips: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
  chip: { paddingHorizontal: 12, paddingVertical: 8, borderRadius: radius.md, backgroundColor: colors.panel2 },
  miniSub: { color: colors.fgMuted, fontSize: font.sm },
  times: { flexDirection: "row", justifyContent: "space-between", marginTop: 6 },
  time: { color: colors.fgMuted, fontSize: font.sm },
  controls: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginTop: 16 },
});
