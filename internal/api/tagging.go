package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/jobs"
	"audioloader/internal/tagging"
)

func getTaggingPage(w http.ResponseWriter, r *http.Request) {
	trackCounts, err := db.TagCounts(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	jobCounts, err := jobs.Counts(tagging.JobTag, tagging.JobCover)
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

	tg := tagging.Default()
	status := components.TaggingStatus{
		Tracks:   trackCounts,
		Jobs:     jobCounts,
		AcoustID: tg.FPCalc != "" && tg.MB.AcoustIDKey != "",
	}
	render(w, r, components.TaggingPage(status, reviews))
}

func getTaggingBadge(w http.ResponseWriter, r *http.Request) {
	counts, err := db.TagCounts(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.Badge(counts[db.TagReview]))
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
	if err := jobs.RetryFailed(tagging.JobTag, tagging.JobCover); err != nil {
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
