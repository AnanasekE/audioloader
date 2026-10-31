import { useRouter } from "expo-router";
import { usePlaylists } from "@/api/hooks";
import { PagedList, Row } from "@/components/ui";

export default function Playlists() {
  const router = useRouter();
  const source = usePlaylists();
  return (
    <PagedList
      source={source}
      keyOf={(p) => p.id}
      empty="No playlists yet."
      render={(p) => (
        <Row
          title={p.name}
          right={p.trackCount !== undefined ? String(p.trackCount) : undefined}
          onPress={() => router.push({ pathname: "/playlist/[id]", params: { id: p.id } })}
        />
      )}
    />
  );
}
