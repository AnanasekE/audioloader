import { useRouter } from "expo-router";
import { useArtists } from "@/api/hooks";
import { Cover, PagedList, Row } from "@/components/ui";

export default function Artists() {
  const router = useRouter();
  const source = useArtists();
  return (
    <PagedList
      source={source}
      keyOf={(a) => a.id}
      empty="No artists yet."
      render={(a) => <Row image={<Cover uri={a.imageUrl} />} title={a.name} onPress={() => router.push({ pathname: "/artist/[id]", params: { id: a.id } })} />}
    />
  );
}
