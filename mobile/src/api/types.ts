// Shapes of the JSON API (plan 21, docs/api.md). All API types live here so
// they are easy to adjust. IDs are strings; durations and positions are seconds.

export type Me = {
  secret: boolean;
  vault: boolean;
  secretSource: boolean;
};

export type ArtistRef = { id: string; name: string };

export type Track = {
  id: string;
  title: string;
  duration: number;
  streamUrl: string;
  imageUrl: string;
  artists: ArtistRef[];
  // episodes only
  position?: number;
  finished?: boolean;
};

export type Chapter = { title: string; start: number };

export type ReleaseRef = { id: string; title: string };
export type TagRef = { id?: string; name: string };

export type TrackDetail = Track & {
  releases: ReleaseRef[];
  tags: TagRef[];
  description: string;
  chapters: Chapter[];
};

export type Page<T> = { items: T[]; total: number };

export type Artist = {
  id: string;
  name: string;
  imageUrl?: string;
};

export type ArtistDetail = Artist & {
  tracks: Track[];
  releases?: Release[]; // normal mode only
};

export type Release = {
  id: string;
  title: string;
  imageUrl: string;
  artists: ArtistRef[];
  year?: number;
};

export type ReleaseDetail = Release & { tracks: Track[] };

export type Playlist = {
  id: string;
  name: string;
  trackCount?: number;
};

export type PlaylistDetail = Playlist & { tracks: Track[] };

export type Show = {
  id: string;
  title: string;
  imageUrl: string;
  episodeCount?: number;
};

export type ShowDetail = Show & { episodes: Track[] };

export type SearchResults = {
  tracks: Track[];
  artists: Artist[];
  releases: Release[];
  playlists: Playlist[];
  shows: Show[];
};

export type ApiErrorBody = { error: string };

// Body of POST /api/tracks/{id}/position (already on the server).
export type PositionBody = { seconds: number; finished: boolean };
