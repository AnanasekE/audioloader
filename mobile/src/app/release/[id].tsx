import { useLocalSearchParams } from "expo-router";
import { useRelease } from "@/api/hooks";
import { TrackListDetail } from "@/components/detail";

export default function ReleaseScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const q = useRelease(id);
  return (
    <TrackListDetail
      title={q.data?.title}
      subtitle={q.data?.artists.map((a) => a.name).join(", ")}
      imageUrl={q.data?.imageUrl}
      tracks={q.data?.tracks}
      loading={q.isLoading}
      error={q.error}
    />
  );
}
