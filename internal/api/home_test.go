package api

import (
	"net/http"
	"strings"
	"testing"

	"audioloader/internal/db"
)

func TestHomePage(t *testing.T) {
	a := newModeApp(t)
	rec := a.do(false, "GET", "/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d", rec.Code)
	}
	b := rec.Body.String()
	for _, want := range []string{
		">Inbox</h2>", "1 new release waiting", `href="/inbox"`, // what is waiting
		"Continue listening", "ZZPUBLIC Episode", // the started episode
		"Recently added", "ZZPUBLIC Album", // what reached the library last
	} {
		if !strings.Contains(b, want) {
			t.Errorf("the home page lacks %q", want)
		}
	}
	// the nav offers it, and marks it as the page we are on
	if !strings.Contains(b, `href="/" aria-current="page"`) {
		t.Error("the nav has no current Home link")
	}
}

// the inbox section is the count, so nothing waiting means no section
func TestHomePageWithAnEmptyInbox(t *testing.T) {
	a := newModeApp(t)
	for _, r := range inboxReleases(t) {
		if err := db.IgnoreInboxRelease(r.ReleaseGroupMBID); err != nil {
			t.Fatal(err)
		}
	}
	b := a.do(false, "GET", "/", nil).Body.String()
	if strings.Contains(b, "waiting") || strings.Contains(b, ">Inbox</h2>") {
		t.Error("the home page shows an inbox section with nothing in it")
	}
	// the rest of the page is still there
	if !strings.Contains(b, "Recently added") {
		t.Error("the home page lost its recently added section")
	}
}

// a library whose releases are all deleted still has its tracks to show
func TestHomePageWithoutReleases(t *testing.T) {
	a := newModeApp(t)
	releases, err := db.ListReleases(db.Normal)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range releases {
		if err := db.DeleteRelease(r.ID.String()); err != nil {
			t.Fatal(err)
		}
	}
	b := a.do(false, "GET", "/", nil).Body.String()
	if !strings.Contains(b, "ZZPUBLIC Song 1") {
		t.Error("the home page lists no tracks where there are no releases")
	}
}

func TestHomePageIsNormalModeOnly(t *testing.T) {
	a := newModeApp(t)
	rec := a.do(true, "GET", "/", nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/tracks" {
		t.Errorf("secret mode GET / = %d to %q, want 303 to /tracks", rec.Code, rec.Header().Get("Location"))
	}
	// and secret mode is not offered a page it cannot open
	if b := a.do(true, "GET", "/tracks", nil).Body.String(); strings.Contains(b, `href="/" aria-current`) || strings.Contains(b, ">Home<") {
		t.Error("the secret nav offers Home")
	}
}

func inboxReleases(t *testing.T) []*db.InboxRelease {
	t.Helper()
	releases, err := db.ListInbox(db.InboxFilter{Status: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) == 0 {
		t.Fatal("the fixture has nothing in the inbox")
	}
	return releases
}
