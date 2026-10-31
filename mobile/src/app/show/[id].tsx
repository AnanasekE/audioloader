import { useLocalSearchParams } from "expo-router";
import { useShow } from "@/api/hooks";
import { TrackListDetail } from "@/components/detail";

export default function ShowScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const q = useShow(id);
  return <TrackListDetail title={q.data?.title} imageUrl={q.data?.imageUrl} tracks={q.data?.episodes} loading={q.isLoading} error={q.error} emptyText="No episodes." />;
}
