package db

import (
	"io"
	"os"
	"path/filepath"
	"time"
	"uuid"

	"audioloader/internal/audio"
)

func ListTracks() ([]*audio.Track, error) {
	query1 := `SELECT uuid, title, duration_seconds, img_url FROM tracks;`
	rows, err := DB.Query(query1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tracks := make([]*audio.Track, 0)
	var duration_sec int
	for rows.Next() {
		var track audio.Track
		if err := rows.Scan(&track.ID, &track.Title, &duration_sec, &track.ImgURL); err != nil {
			return nil, err
		}
		track.Duration = time.Duration(duration_sec) * time.Second
		tracks = append(tracks, &track)
	}
	return tracks, nil
}

func ListArtists() ([]*audio.Artist, error) {
	query1 := `SELECT uuid, name, img_url FROM artists;`
	rows, err := DB.Query(query1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	artists := make([]*audio.Artist, 0)
	for rows.Next() {
		var artist audio.Artist
		if err := rows.Scan(&artist.ID, &artist.Name, &artist.ImgURL); err != nil {
			return nil, err
		}
		artists = append(artists, &artist)
	}
	return artists, nil
}

func AddArtist(name string) error {
	query := `
	INSERT INTO ARTISTS(uuid, name) VALUES(?, ?);
	`
	id := uuid.New().String()

	tx, _ := DB.Begin()
	_, err := tx.Exec(query, id, name)
	if err != nil {
		tx.Rollback()
		return err
	}
	err2 := tx.Commit()
	if err2 != nil {
		return err2
	}
	return nil
}

// TracksDir is where uploaded track files are stored as <uuid><ext>.
var TracksDir = "data/tracks"

func AddTrack(artistUUID string, track *audio.Track, file io.ReadSeeker, ext string) error {
	id := uuid.New().String()

	path, err := saveTrackFile(id, file, ext)
	if err != nil {
		return err
	}
	if err := insertTrack(id, artistUUID, track); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

// saveTrackFile writes file to TracksDir as <id><ext> and returns its path.
// On failure, any partially written file is removed.
func saveTrackFile(id string, file io.ReadSeeker, ext string) (string, error) {
	if err := os.MkdirAll(TracksDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(TracksDir, id+ext)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	out, err := os.Create(path)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(out, file)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// insertTrack inserts the track row and its artist mapping in one transaction.
func insertTrack(id, artistUUID string, track *audio.Track) error {
	query1 := `
	INSERT INTO tracks(uuid, title, duration_seconds) VALUES(?, ?, ?);
	`
	query2 := `
	INSERT INTO track_artist_map(track_id, artist_id) VALUES(?, ?);
	`

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(query1, id, track.Title, int(track.Duration.Seconds())); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(query2, id, artistUUID); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func GetTrack(trackUUID string) (*audio.Track, error) {
	query := `SELECT uuid, title, duration_seconds, img_url FROM tracks WHERE uuid = ?;`

	var track audio.Track
	var duration_sec int
	err := DB.QueryRow(query, trackUUID).Scan(&track.ID, &track.Title, &duration_sec, &track.ImgURL)
	if err != nil {
		return nil, err
	}
	track.Duration = time.Duration(duration_sec) * time.Second
	return &track, nil
}
