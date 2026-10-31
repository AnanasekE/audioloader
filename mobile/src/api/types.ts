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
  kind: "music" | "episode";
  duration: number;
  streamUrl: string; // "" for a placeholder without a file
  imageUrl: string; // "" when there is only the placeholder
  artists: ArtistRef[];
  showId: string; // "" for music
  position: number; // episodes: where playback stopped; 0 for music
  finished: boolean;
};

export type Chapter = { start: number; title: string };

export type TrackDetail = Track & {
  releases: Release[];
  tags: string[];
  description: string;
  chapters: Chapter[];
};

export type Page<T> = { items: T[]; total: number };

export type Artist = {
  id: string;
  name: string;
  imageUrl: string;
};

// releases is [] in secret mode; tracks are those on none of the releases.
export type ArtistDetail = Artist & {
  releases: Release[];
  tracks: Track[];
};

export type Release = {
  id: string;
  title: string;
  type: "album" | "ep" | "single" | "compilation";
  date: string; // YYYY, YYYY-MM, YYYY-MM-DD or ""
  imageUrl: string;
  artists: ArtistRef[];
};

// a track's own position is a playback position, so the tracklist wraps it
export type ReleaseTrack = { disc: number; number: number; track: Track };

export type ReleaseDetail = Release & { duration: number; tracks: ReleaseTrack[] };

export type Playlist = {
  id: string;
  name: string;
  imageUrl: string;
  count: number;
  duration: number;
};

export type PlaylistDetail = Playlist & { tracks: Track[] };

export type Show = {
  id: string;
  title: string;
  author: string;
  imageUrl: string;
  count: number;
  unplayed: number;
};

export type ShowDetail = Show & { episodes: Track[] }; // newest first

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
