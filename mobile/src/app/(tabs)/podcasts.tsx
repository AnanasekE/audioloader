import { useRouter } from "expo-router";
import { useShows } from "@/api/hooks";
import { Cover, PagedList, Row } from "@/components/ui";

export default function Podcasts() {
  const router = useRouter();
  const source = useShows();
  return (
    <PagedList
      source={source}
      keyOf={(s) => s.id}
      empty="No podcasts yet."
      render={(s) => (
        <Row
          image={<Cover uri={s.imageUrl} />}
          title={s.title}
          right={s.episodeCount !== undefined ? String(s.episodeCount) : undefined}
          onPress={() => router.push({ pathname: "/show/[id]", params: { id: s.id } })}
        />
      )}
    />
  );
}
