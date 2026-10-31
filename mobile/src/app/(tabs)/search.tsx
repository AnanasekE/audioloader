import { useRouter } from "expo-router";
import { useEffect, useState } from "react";
import { ActivityIndicator, ScrollView, StyleSheet, TextInput, View } from "react-native";
import { useSearch } from "@/api/hooks";
import { Cover, Message, Row, SectionTitle, TrackRow } from "@/components/ui";
import { playFrom } from "@/player";
import { colors, font, radius } from "@/theme";

export default function Search() {
  const router = useRouter();
  const [text, setText] = useState("");
  const [q, setQ] = useState("");
  // wait for a pause in typing before asking the server
  useEffect(() => {
    const t = setTimeout(() => setQ(text.trim()), 300);
    return () => clearTimeout(t);
  }, [text]);
  const res = useSearch(q);
  const d = res.data;
  const nothing = d && !d.tracks.length && !d.artists.length && !d.releases.length && !d.playlists.length && !d.shows.length;

  return (
    <View style={styles.root}>
      <TextInput
        style={styles.input}
        value={text}
        onChangeText={setText}
        placeholder="Search tracks, artists, podcasts"
        placeholderTextColor={colors.fgSubtle}
        autoCapitalize="none"
        autoCorrect={false}
        clearButtonMode="while-editing"
      />
      {res.isFetching && !d ? <ActivityIndicator color={colors.accent} style={{ margin: 16 }} /> : null}
      {res.error ? <Message error>{res.error.message}</Message> : null}
      {nothing ? <Message>Nothing found.</Message> : null}
      {d ? (
        <ScrollView keyboardShouldPersistTaps="handled">
          {d.tracks.length ? <SectionTitle>Tracks</SectionTitle> : null}
          {d.tracks.map((t, i) => (
            <TrackRow key={t.id} track={t} onPress={() => playFrom(d.tracks, i)} />
          ))}
          {d.artists.length ? <SectionTitle>Artists</SectionTitle> : null}
          {d.artists.map((a) => (
            <Row key={a.id} image={<Cover uri={a.imageUrl} />} title={a.name} onPress={() => router.push({ pathname: "/artist/[id]", params: { id: a.id } })} />
          ))}
          {d.releases.length ? <SectionTitle>Releases</SectionTitle> : null}
          {d.releases.map((r) => (
            <Row key={r.id} image={<Cover uri={r.imageUrl} />} title={r.title} onPress={() => router.push({ pathname: "/release/[id]", params: { id: r.id } })} />
          ))}
          {d.playlists.length ? <SectionTitle>Playlists</SectionTitle> : null}
          {d.playlists.map((p) => (
            <Row key={p.id} title={p.name} onPress={() => router.push({ pathname: "/playlist/[id]", params: { id: p.id } })} />
          ))}
          {d.shows.length ? <SectionTitle>Podcasts</SectionTitle> : null}
          {d.shows.map((s) => (
            <Row key={s.id} image={<Cover uri={s.imageUrl} />} title={s.title} onPress={() => router.push({ pathname: "/show/[id]", params: { id: s.id } })} />
          ))}
        </ScrollView>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: colors.bg },
  input: {
    backgroundColor: colors.panel,
    borderColor: colors.line,
    borderWidth: 1,
    borderRadius: radius.md,
    color: colors.fg,
    fontSize: font.lg,
    padding: 10,
    margin: 12,
  },
});
