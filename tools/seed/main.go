// Command seed fills an empty database with demo data for screenshots.
// It goes through the internal/db functions, so every rule they enforce
// holds for the demo data too.
//
//	go run ./tools/seed -db tmp/shots/run/database.db -files tmp/shots/run/data
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"time"
	"uuid"

	"audioloader/internal/audio"
	"audioloader/internal/db"
)

var (
	nextWav int
	normal  = db.Normal
	secret  = db.Secret
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func mustv[T any](v T, err error) T {
	must(err)
	return v
}

func main() {
	dbPath := flag.String("db", "", "database file to fill (created if missing)")
	files := flag.String("files", "", "directory for tracks, covers and pictures")
	flag.Parse()
	if *dbPath == "" || *files == "" {
		log.Fatal("usage: seed -db <path> -files <dir>")
	}
	must(os.MkdirAll(filepath.Dir(*dbPath), 0o755))
	d, err := db.Open(*dbPath)
	must(err)
	must(db.Migrate(d))
	db.DB = d
	defer d.Close()
	db.TracksDir = filepath.Join(*files, "tracks")
	db.CoversDir = filepath.Join(*files, "covers")
	db.ArtistImagesDir = filepath.Join(*files, "artists")
	db.TrackImagesDir = filepath.Join(*files, "track-images")

	var n int
	must(d.QueryRow(`SELECT (SELECT COUNT(*) FROM tracks) + (SELECT COUNT(*) FROM artists);`).Scan(&n))
	if n > 0 {
		log.Fatal("database isn't empty; refusing to seed it")
	}

	seedLibrary()
	seedSecret()
	fmt.Println("seeded", *dbPath)
}

type albumSpec struct {
	title, date string
	typ         audio.ReleaseType
	artist      string
	discs       [][]string // track titles per disc
}

func seedLibrary() {
	names := []string{"Lumen Park", "The Tidewater Choir", "Mara Voss", "Kite & Sparrow"}
	artists := map[string]*audio.Artist{}
	for _, n := range names {
		a := mustv(db.AddArtist(normal, n))
		artists[n] = a
		must(db.SaveArtistImage(a.ID, pixelPNG(n, 32)))
	}

	specs := []albumSpec{
		{"Glass Harbor", "2023-05-12", audio.ReleaseAlbum, "Lumen Park", [][]string{{"Harbor Lights", "Salt and Static", "Low Orbit", "Paper Lanterns", "Undertow", "Glass Harbor", "Quiet Engines", "Lighthouse Keeper", "Slow Return"}}},
		{"Slow Weather", "2021-10-01", audio.ReleaseAlbum, "The Tidewater Choir", [][]string{{"First Frost", "Grey Meadow", "Rain Scale", "The Long Field", "Kettle Song"}, {"Thaw", "Evening Bells", "Moss", "Slow Weather"}}},
		{"Paper Suns", "2024-03-22", audio.ReleaseAlbum, "Mara Voss", [][]string{{"Paper Suns", "Cassette Summer", "Neon Orchard", "Velvet Hour", "Small Hours", "Northbound", "Afterglow", "Sleepwalking"}}},
		{"Night Shift", "2022-11-04", audio.ReleaseEP, "Kite & Sparrow", [][]string{{"Midnight Bus", "Vending Machine", "Back Alley Choir", "Closing Time"}}},
		{"Low Tide", "2020-07-17", audio.ReleaseEP, "Lumen Park", [][]string{{"Low Tide", "Driftwood", "Mooring"}}},
		{"Postcards", "2024-09-06", audio.ReleaseSingle, "Mara Voss", [][]string{{"Postcards"}}},
		{"Hold the Line", "2023-01-20", audio.ReleaseSingle, "Kite & Sparrow", [][]string{{"Hold the Line"}}},
		{"Winter Lullaby", "2019-12-01", audio.ReleaseSingle, "The Tidewater Choir", [][]string{{"Winter Lullaby"}}},
	}
	var all []string
	trackIDs := map[string]string{}
	for _, s := range specs {
		a := artists[s.artist]
		rel := mustv(db.CreateRelease(s.title, s.typ, s.date, []string{a.ID.String()}))
		must(db.SaveCoverImage(rel.ID, pixelPNG(s.title, 64)))
		for di, disc := range s.discs {
			for pi, title := range disc {
				t := &audio.Track{Title: title, Duration: time.Duration(150+(len(title)*17+pi*23)%170) * time.Second}
				r := mustv(db.AddTrack(normal, []string{a.ID.String()}, t, wav(), ".wav",
					&db.ReleaseLink{ReleaseID: rel.ID.String(), Disc: di + 1, Position: pi + 1}, ""))
				all = append(all, r.TrackID)
				trackIDs[title] = r.TrackID
			}
		}
	}
	// a track that came out as a single before the album: "Also on"
	lp := artists["Lumen Park"]
	single := mustv(db.CreateRelease("Harbor Lights", audio.ReleaseSingle, "2023-03-03", []string{lp.ID.String()}))
	must(db.SaveCoverImage(single.ID, pixelPNG("Harbor Lights single", 64)))
	must(db.PlaceTrack(trackIDs["Harbor Lights"], db.ReleaseLink{ReleaseID: single.ID.String(), Disc: 1, Position: 1}))
	r := mustv(db.AddTrack(normal, []string{lp.ID.String()}, &audio.Track{Title: "Harbor Lights (Demo)", Duration: 187 * time.Second}, wav(), ".wav",
		&db.ReleaseLink{ReleaseID: single.ID.String(), Disc: 1, Position: 2}, ""))
	all = append(all, r.TrackID)

	// playlists
	late := mustv(db.CreatePlaylist(normal, "Late night"))
	run := mustv(db.CreatePlaylist(normal, "Morning run"))
	for i, id := range all {
		if i%4 == 0 {
			must(db.AppendToPlaylist(normal, late.ID.String(), id))
		}
		if i%5 == 1 {
			must(db.AppendToPlaylist(normal, run.ID.String(), id))
		}
	}

	// a show
	show := mustv(db.ResolveShow(normal, "The Long Tape", "Ines Calder"))
	var eps []string
	for i, title := range []string{"The sound of an empty room", "Field recording basics", "Why tape hiss is warm", "Listener mail, mostly"} {
		id := mustv(db.AddEpisode(normal, show.ID.String(), &audio.Track{
			Title: title, Duration: time.Duration(24+i*9) * time.Minute,
			Published: fmt.Sprintf("2024-0%d-1%d", 9-i, i+2),
		}, wav(), ".wav"))
		eps = append(eps, id)
	}
	must(db.SavePosition(eps[1], 14*60, false))
	must(db.SavePosition(eps[3], 30*60, true))

	// the inbox: new and ignored releases of followed artists
	type inb struct {
		rg, title, typ, date, artist string
		tracks                       int
	}
	inbox := []inb{
		{"rg-ember", "Ember Letters", "album", "2025-02-14", "Lumen Park", 6},
		{"rg-1", "Tidal Room", "album", "2025-04-18", "Lumen Park", 10},
		{"rg-2", "Choir at Dusk (Live)", "album", "2025-01-10", "The Tidewater Choir", 12},
		{"rg-3", "Lowlight", "ep", "2025-03-07", "Mara Voss", 5},
		{"rg-4", "Satellite", "single", "2025-05-02", "Kite & Sparrow", 1},
		{"rg-5", "Gone Fishing", "single", "2024-12-13", "Kite & Sparrow", 2},
		{"rg-6", "Early Demos", "album", "2018-06-01", "Mara Voss", 9},
		{"rg-7", "Remixes Vol. 1", "compilation", "2024-08-30", "Lumen Park", 8},
	}
	for _, i := range inbox {
		a := artists[i.artist]
		data := db.InboxData{Artists: []db.MatchArtist{{Name: i.artist}}}
		for p := 1; p <= i.tracks; p++ {
			data.Tracks = append(data.Tracks, db.InboxTrack{
				Disc: 1, Position: p, Title: fmt.Sprintf("%s, part %d", i.title, p),
				RecordingMBID: uuid.New().String(), LengthMs: (170 + p*13) * 1000,
			})
		}
		must(db.AddInboxRelease(db.InboxRelease{ReleaseGroupMBID: i.rg, ArtistID: a.ID.String(), Title: i.title, Type: i.typ, Date: i.date, Data: data}))
	}
	must(db.IgnoreInboxRelease("rg-6"))
	must(db.IgnoreInboxRelease("rg-7"))
	// "Ember Letters" is confirmed: its tracks are placeholders with downloads
	_, placeholders, err := db.ConfirmInboxRelease("rg-ember")
	must(err)
	states := []struct {
		status, source string
		progress       float64
		err            string
	}{
		{db.DownloadDownloading, "youtube", 0.42, ""},
		{db.DownloadSearching, "", 0, ""},
		{db.DownloadNeedsPick, "", 0, ""},
		{db.DownloadFailed, "youtube", 0, "yt-dlp exited with status 1: video unavailable"},
		{db.DownloadNotFound, "", 0, ""},
		{db.DownloadQueued, "", 0, ""},
	}
	for i, id := range placeholders {
		s := states[i%len(states)]
		must(db.SetDownload(id, s.status, s.source, s.progress, s.err))
	}

	// follows, marked as just checked so nothing goes looking for them
	for _, n := range []string{"Lumen Park", "Mara Voss", "Kite & Sparrow"} {
		a := artists[n]
		must(db.SetFollow(a.ID.String(), uuid.New().String(), db.DefaultFilters()))
		must(db.MarkFollowChecked(a.ID.String()))
	}

	// tagging reviews and a cover suggestion
	for i, title := range []string{"Salt and Static", "Cassette Summer", "Vending Machine"} {
		id := trackIDs[title]
		must(db.SaveMatches(id, []db.Match{
			{Source: "acoustid", Score: 0.91 - float64(i)*0.07, RecordingID: uuid.New().String(), Title: title, LengthMs: 201000,
				Artists:      []db.MatchArtist{{ID: uuid.New().String(), Name: "Lumen Park"}},
				ReleaseTitle: "Glass Harbor", ReleaseType: "album", ReleaseDate: "2023"},
			{Source: "search", Score: 0.62, RecordingID: uuid.New().String(), Title: title + " (Live)", LengthMs: 233000,
				Artists:      []db.MatchArtist{{ID: uuid.New().String(), Name: "Lumen Park"}},
				ReleaseTitle: "Live at the Dock", ReleaseType: "album", ReleaseDate: "2024"},
		}, db.TagReview))
	}
	must(db.SaveCoverSuggestion(trackIDs["Hold the Line"], db.CoverSuggestion{Title: "Hold the Line", Artists: "Toto"}))
}

func seedSecret() {
	creators := []string{"nightbloom", "quietfield", "tape ghost", "pale harbor", "dusk radio"}
	var ids []string
	for _, c := range creators {
		ids = append(ids, mustv(db.AddArtist(secret, c)).ID.String())
	}
	tags := []string{"ambient", "rain", "lofi", "piano", "field recording", "drone", "study", "sleep", "night", "city",
		"forest", "ocean", "cassette", "vinyl", "modular", "guitar", "choir", "tape", "noise", "slow",
		"warm", "winter", "storm", "morning", "train"}
	titles := []string{"Rain on tin", "Station at 4am", "Tape loop no. 3", "Night bus window", "Cold room piano", "Harbor fog", "Sleep machine", "Forest floor", "Warm static",
		"Slow train", "Choir in a stairwell", "Winter drone", "Morning kettle", "Storm cellar", "Modular lullaby", "Cassette hiss study", "City asleep", "Ocean in a jar"}
	for i, title := range titles {
		var tg []string
		for k := 0; k < 1+i%5; k++ {
			tg = append(tg, tags[(i*3+k*7)%len(tags)])
		}
		t := &audio.Track{Title: title, Duration: time.Duration(120+i*31%400) * time.Second, Tags: tg,
			Description: "Found in a long folder of unlabelled recordings."}
		mustv(db.AddTrack(secret, []string{ids[i%len(ids)]}, t, wav(), ".wav", nil, ""))
	}
}

// wav returns a 2-second silent mono WAV. Each call differs in its first
// samples, since the library refuses two identical files.
func wav() *bytes.Reader {
	nextWav++
	const rate = 8000
	data := bytes.Repeat([]byte{128}, 2*rate)
	binary.LittleEndian.PutUint32(data, uint32(nextWav)*2654435761)
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+len(data)))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, []any{uint32(16), uint16(1), uint16(1), uint32(rate), uint32(rate), uint16(1), uint16(8)})
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(len(data)))
	b.Write(data)
	return bytes.NewReader(b.Bytes())
}

// pixelPNG is a small symmetric pixel pattern coloured from seed, scaled to
// size pixels square.
func pixelPNG(seed string, size int) *bytes.Reader {
	h := fnv.New64a()
	h.Write([]byte(seed))
	s := h.Sum64()
	hue := float64(s%360) / 360
	fg := hsl(hue, 0.55, 0.55)
	bg := hsl(hue+0.08, 0.35, 0.22)
	const cells = 8
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	bits := s >> 9
	for cy := 0; cy < cells; cy++ {
		for cx := 0; cx < cells/2; cx++ {
			c := bg
			if bits>>(uint(cy*4+cx))&1 == 1 {
				c = fg
			}
			for _, x := range []int{cx, cells - 1 - cx} {
				for py := cy * size / cells; py < (cy+1)*size/cells; py++ {
					for px := x * size / cells; px < (x+1)*size/cells; px++ {
						img.Set(px, py, c)
					}
				}
			}
		}
	}
	var b bytes.Buffer
	must(png.Encode(&b, img))
	return bytes.NewReader(b.Bytes())
}

func hsl(h, s, l float64) color.RGBA {
	h -= float64(int(h))
	q := l * (1 + s)
	if l >= 0.5 {
		q = l + s - l*s
	}
	p := 2*l - q
	f := func(t float64) uint8 {
		t -= float64(int(t))
		if t < 0 {
			t++
		}
		var v float64
		switch {
		case t < 1.0/6:
			v = p + (q-p)*6*t
		case t < 0.5:
			v = q
		case t < 2.0/3:
			v = p + (q-p)*(2.0/3-t)*6
		default:
			v = p
		}
		return uint8(v * 255)
	}
	return color.RGBA{f(h + 1.0/3), f(h), f(h - 1.0/3), 255}
}
