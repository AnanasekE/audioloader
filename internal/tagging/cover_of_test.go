package tagging

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/jobs"
	"audioloader/internal/mb"
)

const (
	hurtCover    = "c0c0c0c0-0000-4000-8000-000000000001"
	hurtOriginal = "c0c0c0c0-0000-4000-8000-000000000002"
	hurtWork     = "e0e0e0e0-0000-4000-8000-000000000001"
)

// coverTagger answers lookups of the cover recording, the original recording
// and the work from fixtures. workFile is the fixture served for the work.
func coverTagger(t *testing.T, workFile string) *Tagger {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		files := map[string]string{
			"/ws/2/recording/" + hurtCover:    "recording_cover.json",
			"/ws/2/recording/" + hurtOriginal: "recording_original.json",
			"/ws/2/work/" + hurtWork:          workFile,
		}
		file, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, err := os.ReadFile("../mb/testdata/" + file)
		if err != nil {
			t.Error(err)
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	c := mb.New()
	c.MusicBrainzURL = srv.URL + "/ws/2"
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return &Tagger{MB: c}
}

func TestCoverSuggestion(t *testing.T) {
	for _, workFile := range []string{"work_cover.json", "work_cover_no_credit.json"} {
		t.Run(workFile, func(t *testing.T) {
			dbtest.New(t)
			tg := coverTagger(t, workFile)
			trackID, _ := addTrack(t, "Hurt", "Johnny Cash", 218*time.Second, hurtCover, "Hurt", audio.ReleaseSingle)

			// matching the cover recording queues the lookup of its work
			if err := tg.Tag(context.Background(), trackID); err != nil {
				t.Fatal(err)
			}
			if counts, _ := jobs.Counts(JobCoverOf); counts["queued"] != 1 {
				t.Fatalf("jobs = %v", counts)
			}
			if err := tg.SuggestCoverOf(context.Background(), trackID, hurtWork, hurtCover); err != nil {
				t.Fatal(err)
			}
			sugg, err := db.PendingCoverSuggestions(db.All)
			if err != nil || len(sugg) != 1 {
				t.Fatalf("suggestions = %+v, %v", sugg, err)
			}
			if s := sugg[0]; s.Track.ID.String() != trackID || s.String() != "Hurt by Nine Inch Nails" || s.MBID != hurtOriginal {
				t.Fatalf("suggestion = %+v", s)
			}
			// nothing is set until the user accepts
			if tr, _ := db.GetTrack(db.All, trackID); tr.CoverOf != nil {
				t.Fatalf("suggestion applied on its own: %+v", tr.CoverOf)
			}
			if err := db.ApplyCoverSuggestion(db.All, trackID); err != nil {
				t.Fatal(err)
			}
			tr, _ := db.GetTrack(db.All, trackID)
			if tr.CoverOf == nil || tr.CoverOf.String() != "Hurt by Nine Inch Nails" || tr.CoverOf.MBID != hurtOriginal {
				t.Fatalf("cover of = %+v", tr.CoverOf)
			}
			if len(tr.Artists) != 1 || tr.Artists[0].Name != "Johnny Cash" {
				t.Fatalf("the cover's credits changed: %+v", tr.Artists)
			}
		})
	}
}

func TestCoverSuggestionNeverOverwrites(t *testing.T) {
	dbtest.New(t)
	tg := coverTagger(t, "work_cover.json")
	trackID, _ := addTrack(t, "Hurt", "Johnny Cash", 218*time.Second, hurtCover, "Hurt", audio.ReleaseSingle)
	tr, _ := db.GetTrack(db.All, trackID)
	artists := []string{tr.Artists[0].ID.String()}
	set := func(c db.CoverEdit) {
		t.Helper()
		if err := db.UpdateTrack(db.All, trackID, db.TrackEdit{Title: "Hurt", Kind: audio.KindMusic, ArtistIDs: artists, Cover: &c}); err != nil {
			t.Fatal(err)
		}
	}

	// already set: no suggestion is stored
	set(db.CoverEdit{Title: "My Own Pick"})
	if err := tg.SuggestCoverOf(context.Background(), trackID, hurtWork, hurtCover); err != nil {
		t.Fatal(err)
	}
	if sugg, _ := db.PendingCoverSuggestions(db.All); len(sugg) != 0 {
		t.Fatalf("suggested over a set original: %+v", sugg)
	}

	// set after the suggestion was made: it can no longer be applied
	set(db.CoverEdit{})
	if err := tg.SuggestCoverOf(context.Background(), trackID, hurtWork, hurtCover); err != nil {
		t.Fatal(err)
	}
	set(db.CoverEdit{Title: "My Own Pick"})
	if sugg, _ := db.PendingCoverSuggestions(db.All); len(sugg) != 0 {
		t.Fatalf("listed for a track with an original: %+v", sugg)
	}
	if err := db.ApplyCoverSuggestion(db.All, trackID); err != db.ErrCoverSet {
		t.Fatalf("apply over a set original = %v", err)
	}
	if tr, _ := db.GetTrack(db.All, trackID); tr.CoverOf.Title != "My Own Pick" {
		t.Fatalf("cover of = %+v", tr.CoverOf)
	}
}

func TestCoverSuggestionReject(t *testing.T) {
	dbtest.New(t)
	tg := coverTagger(t, "work_cover.json")
	trackID, _ := addTrack(t, "Hurt", "Johnny Cash", 218*time.Second, hurtCover, "Hurt", audio.ReleaseSingle)
	tg.SuggestCoverOf(context.Background(), trackID, hurtWork, hurtCover)
	if err := db.RejectCoverSuggestion(db.All, trackID); err != nil {
		t.Fatal(err)
	}
	// not proposed again, and not applicable
	tg.SuggestCoverOf(context.Background(), trackID, hurtWork, hurtCover)
	if sugg, _ := db.PendingCoverSuggestions(db.All); len(sugg) != 0 {
		t.Fatalf("rejected came back: %+v", sugg)
	}
	if err := db.ApplyCoverSuggestion(db.All, trackID); err != db.ErrNotFound {
		t.Fatalf("apply rejected = %v", err)
	}
}

func TestNoCoverSuggestionForOriginal(t *testing.T) {
	dbtest.New(t)
	tg := coverTagger(t, "work_cover.json")
	trackID, _ := addTrack(t, "Hurt", "Nine Inch Nails", 373*time.Second, hurtOriginal, "Hurt", audio.ReleaseSingle)
	if err := tg.Tag(context.Background(), trackID); err != nil {
		t.Fatal(err)
	}
	if counts, _ := jobs.Counts(JobCoverOf); counts["queued"] != 0 {
		t.Fatalf("an original queued a lookup: %v", counts)
	}
}

// queuedSong adds the cover recording as a placeholder, like Add does.
func queuedSong(t *testing.T) string {
	t.Helper()
	id, _, _, err := db.AddSong(db.NewSong{
		RecordingMBID: hurtCover, Title: "Hurt", LengthMs: 218000,
		Artists: []db.MatchArtist{{Name: "Johnny Cash"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestApplyCoverOfText(t *testing.T) {
	dbtest.New(t)
	tg := coverTagger(t, "work_cover.json")
	id := queuedSong(t)
	if err := tg.ApplyCoverOf(context.Background(), id, hurtWork, hurtCover); err != nil {
		t.Fatal(err)
	}
	tr, _ := db.GetTrack(db.All, id)
	if tr.CoverOf == nil || tr.CoverOf.String() != "Hurt by Nine Inch Nails" || tr.CoverOf.MBID != hurtOriginal || tr.CoverOf.TrackID != "" {
		t.Fatalf("cover of = %+v", tr.CoverOf)
	}
	if sugg, _ := db.PendingCoverSuggestions(db.All); len(sugg) != 0 {
		t.Fatalf("applied songs get no suggestion: %+v", sugg)
	}
}

func TestApplyCoverOfLinksLibraryTrack(t *testing.T) {
	dbtest.New(t)
	tg := coverTagger(t, "work_cover.json")
	origID, _ := addTrack(t, "Hurt", "Nine Inch Nails", 373*time.Second, hurtOriginal, "The Downward Spiral", audio.ReleaseAlbum)
	id := queuedSong(t)
	if err := tg.ApplyCoverOf(context.Background(), id, hurtWork, hurtCover); err != nil {
		t.Fatal(err)
	}
	tr, _ := db.GetTrack(db.All, id)
	if tr.CoverOf == nil || tr.CoverOf.TrackID != origID {
		t.Fatalf("cover of = %+v", tr.CoverOf)
	}
}

func TestApplyCoverOfNeverOverwritesOrTouchesSecret(t *testing.T) {
	dbtest.New(t)
	tg := coverTagger(t, "work_cover.json")
	id := queuedSong(t)
	tr, _ := db.GetTrack(db.All, id)
	set := db.CoverEdit{Title: "My Own Pick"}
	err := db.UpdateTrack(db.All, id, db.TrackEdit{Title: "Hurt", Kind: audio.KindMusic, ArtistIDs: []string{tr.Artists[0].ID.String()}, Cover: &set})
	if err != nil {
		t.Fatal(err)
	}
	if err := tg.ApplyCoverOf(context.Background(), id, hurtWork, hurtCover); err != nil {
		t.Fatal(err)
	}
	if tr, _ := db.GetTrack(db.All, id); tr.CoverOf == nil || tr.CoverOf.Title != "My Own Pick" {
		t.Fatalf("overwritten: %+v", tr.CoverOf)
	}
	if err := db.ApplyCover(id, db.CoverEdit{Title: "x"}); err != db.ErrCoverSet {
		t.Fatalf("ApplyCover over a set original = %v", err)
	}

	secretID, _ := addTrack(t, "Hurt", "Someone Else", 218*time.Second, hurtCover+"x", "Hurt", audio.ReleaseSingle)
	if err := db.SetSecret(secretID, true); err != nil {
		t.Fatal(err)
	}
	if err := tg.ApplyCoverOf(context.Background(), secretID, hurtWork, hurtCover); err != nil {
		t.Fatal(err)
	}
	if st, _ := db.GetTrack(db.All, secretID); st.CoverOf != nil {
		t.Fatalf("secret track got a cover: %+v", st.CoverOf)
	}
}
