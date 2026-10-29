package api

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"audioloader/internal/db"
	"audioloader/internal/dbtest"
)

func TestRemoveReleaseRoute(t *testing.T) {
	dbtest.New(t)
	h := downloadsHandler(t)
	trackID := placeholder(t)
	tr, err := db.GetTrack(db.All, trackID)
	if err != nil || len(tr.Releases) != 1 {
		t.Fatalf("track = %+v, err = %v", tr, err)
	}
	rel := tr.Releases[0].ID.String()

	body := do(h, "GET", "/releases/"+rel, nil).Body.String()
	if !strings.Contains(body, "Remove release") || !strings.Contains(body, "1 track goes") {
		t.Fatalf("release page lacks the remove button or its confirm text")
	}

	rec := do(h, "POST", "/ui/releases/"+rel+"/remove", nil)
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Location") == "" || !strings.Contains(rec.Header().Get("HX-Location"), "/releases") {
		t.Fatalf("remove = %d, HX-Location %q", rec.Code, rec.Header().Get("HX-Location"))
	}
	if _, err := db.GetTrack(db.All, trackID); !errors.Is(err, db.ErrNotFound) {
		t.Errorf("track err = %v", err)
	}
	if _, err := db.GetDownload(db.All, trackID); !errors.Is(err, db.ErrNotFound) {
		t.Errorf("download err = %v", err)
	}
	if body := do(h, "GET", "/inbox?tab=ignored", nil).Body.String(); !strings.Contains(body, "Mezzanine") {
		t.Error("Ignored tab doesn't list Mezzanine")
	}
	if rec := do(h, "POST", "/ui/releases/"+rel+"/remove", nil); rec.Code != http.StatusNotFound {
		t.Errorf("second remove = %d, want 404", rec.Code)
	}
}
