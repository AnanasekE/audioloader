import { useLocalSearchParams } from "expo-router";
import { usePlaylist } from "@/api/hooks";
import { TrackListDetail } from "@/components/detail";

export default function PlaylistScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const q = usePlaylist(id);
  return <TrackListDetail title={q.data?.name} tracks={q.data?.tracks} loading={q.isLoading} error={q.error} queueAll emptyText="This playlist is empty." />;
}
