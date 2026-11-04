package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/jobs"
	"audioloader/internal/mb"
	"audioloader/internal/tagging"
)

func getTaggingPage(w http.ResponseWriter, r *http.Request) {
	trackCounts, err := db.TagCounts(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	jobCounts, err := jobs.Counts(tagging.JobTag, tagging.JobCover, tagging.JobCoverOf, tagging.JobCoverOfApply, tagging.JobCoverBackfill)
	if err != nil {
		serverError(w, err)
		return
	}
	pending, err := db.PendingMatches(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}

	var reviews []components.TaggingReview
	for _, p := range pending {
		if n := len(reviews); n > 0 && reviews[n-1].Track.ID.String() == p.TrackID {
			reviews[n-1].Candidates = append(reviews[n-1].Candidates, p.Match)
			continue
		}
		t, err := db.GetTrack(viewer(r), p.TrackID)
		if err != nil {
			serverError(w, err)
			return
		}
		reviews = append(reviews, components.TaggingReview{Track: t, Candidates: []db.Match{p.Match}})
	}

	suggestions, err := db.ArtistSuggestions(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}

	covers, err := db.PendingCoverSuggestions(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}

	moves, err := db.MoveSuggestions(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}

	tg := tagging.Default()
	status := components.TaggingStatus{
		Tracks:   trackCounts,
		Jobs:     jobCounts,
		AcoustID: tg.FPCalc != "" && tg.MB.AcoustIDKey != "",
	}
	render(w, r, components.TaggingPage(status, reviews, suggestions, covers, moves))
}

func getTaggingBadge(w http.ResponseWriter, r *http.Request) {
	counts, err := db.TagCounts(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.Badge(counts[db.TagReview]))
}

// getMoreBadge is the dot on the phone tab bar's Library tab: tagging
// reviews plus downloads waiting for a pick.
func getMoreBadge(w http.ResponseWriter, r *http.Request) {
	counts, err := db.TagCounts(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	list, err := db.ListDownloads(viewer(r), db.DownloadNeedsPick)
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.BadgeDot(counts[db.TagReview]+len(list)))
}

// runTagging queues every track that was never tagged or had no match.
func runTagging(w http.ResponseWriter, r *http.Request) {
	ids, err := db.UntaggedTrackIDs()
	if err != nil {
		serverError(w, err)
		return
	}
	for _, id := range ids {
		if err := tagging.Enqueue(id); err != nil {
			serverError(w, err)
			return
		}
	}
	hxLocation(w, "/tagging")
}

func retryTagging(w http.ResponseWriter, r *http.Request) {
	if err := jobs.RetryFailed(tagging.JobTag, tagging.JobCover, tagging.JobCoverOf, tagging.JobCoverOfApply, tagging.JobCoverBackfill); err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/tagging")
}

func applyTagMatch(w http.ResponseWriter, r *http.Request) {
	trackID, ok := pathUUID(w, r, "track_id")
	if !ok || !visibleTrack(w, r, trackID.String()) {
		return
	}
	recID, ok := pathUUID(w, r, "recording_id")
	if !ok {
		return
	}
	err := tagging.Apply(trackID.String(), recID.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/tagging")
}

func rejectTagMatch(w http.ResponseWriter, r *http.Request) {
	trackID, ok := pathUUID(w, r, "track_id")
	if !ok || !visibleTrack(w, r, trackID.String()) {
		return
	}
	recID, ok := pathUUID(w, r, "recording_id")
	if !ok {
		return
	}
	err := db.RejectMatch(trackID.String(), recID.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/tagging")
}

// applyAllTagMatches applies each track's best pending match scoring at least min percent.
func applyAllTagMatches(w http.ResponseWriter, r *http.Request) {
	minPct, err := strconv.Atoi(r.FormValue("min"))
	if err != nil || minPct < 50 || minPct > 100 {
		http.Error(w, "min must be a percentage from 50 to 100", http.StatusBadRequest)
		return
	}
	pending, err := db.PendingMatches(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	done := map[string]bool{}
	for _, p := range pending {
		// pending is ordered best first per track
		if done[p.TrackID] {
			continue
		}
		done[p.TrackID] = true
		if p.Score*100 < float64(minPct) {
			continue
		}
		if err := tagging.Apply(p.TrackID, p.RecordingID); err != nil {
			log.Println("apply tag match:", err)
		}
	}
	hxLocation(w, "/tagging")
}

func applyCoverSuggestion(w http.ResponseWriter, r *http.Request) {
	trackID, ok := pathUUID(w, r, "track_id")
	if !ok {
		return
	}
	err := db.ApplyCoverSuggestion(viewer(r), trackID.String())
	switch {
	case errors.Is(err, db.ErrNotFound):
		http.NotFound(w, r)
		return
	case errors.Is(err, db.ErrCoverSet):
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	hxLocation(w, "/tagging")
}

func rejectCoverSuggestion(w http.ResponseWriter, r *http.Request) {
	trackID, ok := pathUUID(w, r, "track_id")
	if !ok {
		return
	}
	err := db.RejectCoverSuggestion(viewer(r), trackID.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/tagging")
}

// moveTrack moves a YouTube import onto the release its match is on.
func moveTrack(w http.ResponseWriter, r *http.Request) {
	trackID, ok := pathUUID(w, r, "track_id")
	if !ok || !visibleTrack(w, r, trackID.String()) {
		return
	}
	err := tagging.Move(r.Context(), mb.Default(), trackID.String())
	switch {
	case errors.Is(err, tagging.ErrNoMove), errors.Is(err, db.ErrNotFound):
		http.NotFound(w, r)
		return
	case errors.Is(err, tagging.ErrNotOnRelease), errors.Is(err, mb.ErrNotFound):
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	hxLocation(w, "/tagging")
}

// dismissMove keeps a track where it is and stops offering that release.
func dismissMove(w http.ResponseWriter, r *http.Request) {
	trackID, ok := pathUUID(w, r, "track_id")
	if !ok || !visibleTrack(w, r, trackID.String()) {
		return
	}
	s, err := db.MoveSuggestionFor(viewer(r), trackID.String())
	if err != nil {
		serverError(w, err)
		return
	}
	if s == nil {
		http.NotFound(w, r)
		return
	}
	if err := db.DismissMove(s.TrackID, s.TargetID); err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/tagging")
}
