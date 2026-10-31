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
              <Row key={r.id} image={<Cover uri={r.imageUrl} />} title={r.title} onPress={() => router.push({ pathname: "/release/[id]", params: { id: r.id } })} />
            ))}
            <SectionTitle>Tracks</SectionTitle>
          </View>
        ) : null
      }
    />
  );
}
