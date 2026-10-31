import { useRouter } from "expo-router";
import { useReleases } from "@/api/hooks";
import { Cover, PagedList, Row } from "@/components/ui";

export default function Releases() {
  const router = useRouter();
  const source = useReleases();
  return (
    <PagedList
      source={source}
      keyOf={(r) => r.id}
      empty="No releases yet."
      render={(r) => (
        <Row
          image={<Cover uri={r.imageUrl} />}
          title={r.title}
          subtitle={r.artists.map((a) => a.name).join(", ")}
          onPress={() => router.push({ pathname: "/release/[id]", params: { id: r.id } })}
        />
      )}
    />
  );
}
