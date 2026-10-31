import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { get, type Params } from "./client";
import type {
  Artist,
  ArtistDetail,
  Me,
  Page,
  Playlist,
  PlaylistDetail,
  Release,
  ReleaseDetail,
  SearchResults,
  Show,
  ShowDetail,
  Track,
  TrackDetail,
} from "./types";

// One hook per endpoint. Query keys start with a name; "me" is the only key
// that survives a mode change.
export const meKey = ["me"] as const;

export function useMe() {
  return useQuery({
    queryKey: meKey,
    queryFn: () => get<Me>("/api/v1/me"),
    refetchInterval: 60_000,
  });
}

export const PAGE_SIZE = 50;

// usePagedList loads a {items, total} endpoint page by page.
function usePagedList<T>(name: string, path: string, params: Params = {}) {
  const q = useInfiniteQuery({
    queryKey: [name, path, params],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => get<Page<T>>(path, { ...params, limit: PAGE_SIZE, offset: pageParam }),
    getNextPageParam: (last, all) => {
      const loaded = all.reduce((n, p) => n + p.items.length, 0);
      return last.items.length > 0 && loaded < last.total ? loaded : undefined;
    },
  });
  const items = q.data?.pages.flatMap((p) => p.items) ?? [];
  return {
    items,
    total: q.data?.pages[0]?.total ?? 0,
    isLoading: q.isLoading,
    isRefetching: q.isRefetching && !q.isFetchingNextPage,
    error: q.error,
    refetch: q.refetch,
    hasNextPage: q.hasNextPage,
    isFetchingNextPage: q.isFetchingNextPage,
    fetchNextPage: () => {
      if (q.hasNextPage && !q.isFetchingNextPage) void q.fetchNextPage();
    },
  };
}

export const useTracks = (sort?: string) => usePagedList<Track>("tracks", "/api/v1/tracks", { sort });
export const useArtists = () => usePagedList<Artist>("artists", "/api/v1/artists");
export const useReleases = () => usePagedList<Release>("releases", "/api/v1/releases");
export const usePlaylists = () => usePagedList<Playlist>("playlists", "/api/v1/playlists");
export const useShows = () => usePagedList<Show>("shows", "/api/v1/shows");

export const useTrack = (id: string) =>
  useQuery({ queryKey: ["track", id], queryFn: () => get<TrackDetail>(`/api/v1/tracks/${id}`) });
export const useArtist = (id: string) =>
  useQuery({ queryKey: ["artist", id], queryFn: () => get<ArtistDetail>(`/api/v1/artists/${id}`) });
export const useRelease = (id: string) =>
  useQuery({ queryKey: ["release", id], queryFn: () => get<ReleaseDetail>(`/api/v1/releases/${id}`) });
export const usePlaylist = (id: string) =>
  useQuery({ queryKey: ["playlist", id], queryFn: () => get<PlaylistDetail>(`/api/v1/playlists/${id}`) });
export const useShow = (id: string) =>
  useQuery({ queryKey: ["show", id], queryFn: () => get<ShowDetail>(`/api/v1/shows/${id}`) });

export const useSearch = (q: string) =>
  useQuery({
    queryKey: ["search", q],
    queryFn: () => get<SearchResults>("/api/v1/search", { q }),
    enabled: q.trim().length > 0,
  });
