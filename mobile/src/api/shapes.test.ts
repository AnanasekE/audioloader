import type { Artist, ArtistDetail, Playlist, PlaylistDetail, Release, ReleaseDetail, ReleaseTrack, SearchResults, Show, ShowDetail, Track, TrackDetail } from "./types";

// Samples typed with the app's types (so tsc catches a renamed or missing
// field) and checked against the exact key lists the Go tests fix in
// internal/api/apiv1*_test.go (wantKeys). If the server's shapes change, those
// tests fail first; update types.ts and these lists together.

const keys = (o: object) => Object.keys(o).sort().join(" ");

const track: Track = {
  id: "t1",
  title: "Song",
  kind: "music",
  duration: 180,
  streamUrl: "/api/tracks/t1/stream",
  imageUrl: "/covers/x?v=1",
  artists: [{ id: "a1", name: "Artist" }],
  showId: "",
  position: 0,
  finished: false,
};
const release: Release = { id: "r1", title: "Album", type: "album", date: "2001", imageUrl: "", artists: [{ id: "a1", name: "Artist" }] };
const artist: Artist = { id: "a1", name: "Artist", imageUrl: "" };
const playlist: Playlist = { id: "p1", name: "Mix", imageUrl: "", count: 1, duration: 180 };
const show: Show = { id: "s1", title: "Cast", author: "Me", imageUrl: "", count: 3, unplayed: 2 };

describe("API shapes match the Go tests", () => {
  it("track and its parts", () => {
    expect(keys(track)).toBe("artists duration finished id imageUrl kind position showId streamUrl title");
    expect(keys(track.artists[0])).toBe("id name");
    const detail: TrackDetail = { ...track, releases: [release], tags: ["x"], description: "", chapters: [{ start: 0, title: "Intro" }] };
    expect(keys(detail)).toBe("artists chapters description duration finished id imageUrl kind position releases showId streamUrl tags title");
    expect(keys(detail.chapters[0])).toBe("start title");
  });

  it("release, with a wrapped tracklist", () => {
    expect(keys(release)).toBe("artists date id imageUrl title type");
    const rt: ReleaseTrack = { disc: 1, number: 1, track };
    const detail: ReleaseDetail = { ...release, duration: 180, tracks: [rt] };
    expect(keys(detail)).toBe("artists date duration id imageUrl title tracks type");
    expect(keys(rt)).toBe("disc number track");
  });

  it("artist, playlist, show and search", () => {
    expect(keys(artist)).toBe("id imageUrl name");
    const ad: ArtistDetail = { ...artist, releases: [release], tracks: [track] };
    expect(keys(ad)).toBe("id imageUrl name releases tracks");
    expect(keys(playlist)).toBe("count duration id imageUrl name");
    const pd: PlaylistDetail = { ...playlist, tracks: [track] };
    expect(keys(pd)).toBe("count duration id imageUrl name tracks");
    expect(keys(show)).toBe("author count id imageUrl title unplayed");
    const sd: ShowDetail = { ...show, episodes: [track] };
    expect(keys(sd)).toBe("author count episodes id imageUrl title unplayed");
    const search: SearchResults = { tracks: [], artists: [], releases: [], playlists: [], shows: [] };
    expect(keys(search)).toBe("artists playlists releases shows tracks");
  });
});
