import { useTracks } from "@/api/hooks";
import { PagedList, TrackRow } from "@/components/ui";
import { playFrom } from "@/player";

export default function Tracks() {
  const source = useTracks();
  return (
    <PagedList
      source={source}
      keyOf={(t) => t.id}
      empty="No tracks yet."
      render={(t, i) => <TrackRow track={t} onPress={() => playFrom(source.items, i)} />}
    />
  );
}
