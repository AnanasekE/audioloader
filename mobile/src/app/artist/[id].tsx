import { useLocalSearchParams, useRouter } from "expo-router";
import { View } from "react-native";
import { useArtist } from "@/api/hooks";
import { TrackListDetail } from "@/components/detail";
import { Cover, Row, SectionTitle } from "@/components/ui";

export default function ArtistScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const q = useArtist(id);
  const releases = q.data?.releases ?? [];
  const hasTracks = (q.data?.tracks.length ?? 0) > 0;
  return (
    <TrackListDetail
      title={q.data?.name}
      imageUrl={q.data?.imageUrl}
      tracks={q.data?.tracks}
      loading={q.isLoading}
      error={q.error}
      extra={
        releases.length ? (
          <View>
            <SectionTitle>Releases</SectionTitle>
            {releases.map((r) => (
              <Row key={r.id} image={<Cover uri={r.imageUrl} />} title={r.title} subtitle={[r.type, r.date.slice(0, 4)].filter(Boolean).join(" · ")} onPress={() => router.push({ pathname: "/release/[id]", params: { id: r.id } })} />
            ))}
            {hasTracks ? <SectionTitle>Other tracks</SectionTitle> : null}
          </View>
        ) : null
      }
    />
  );
}
