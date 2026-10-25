package db

import (
	"database/sql"
	"errors"
	"uuid"

	"audioloader/internal/audio"
)

func ListArtists() ([]*audio.Artist, error) {
	rows, err := DB.Query(`SELECT uuid, name, img_url FROM artists;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	artists := make([]*audio.Artist, 0)
	for rows.Next() {
		var a audio.Artist
		if err := rows.Scan(&a.ID, &a.Name, &a.ImgURL); err != nil {
			return nil, err
		}
		artists = append(artists, &a)
	}
	return artists, rows.Err()
}

func AddArtist(name string) (*audio.Artist, error) {
	a := &audio.Artist{ID: uuid.New(), Name: name}
	_, err := DB.Exec(
		`INSERT INTO artists(uuid, name) VALUES(?, ?);`,
		a.ID.String(), a.Name,
	)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ResolveArtists maps each name to an artist ID, matching existing artists
// case-insensitively and creating the missing ones, all in one transaction.
func ResolveArtists(names []string) (map[string]string, error) {
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ids := make(map[string]string, len(names))
	for _, name := range names {
		if _, done := ids[name]; done {
			continue
		}
		var id string
		err := tx.QueryRow(`SELECT uuid FROM artists WHERE lower(name) = lower(?) LIMIT 1;`, name).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			id = uuid.New().String()
			_, err = tx.Exec(`INSERT INTO artists(uuid, name) VALUES(?, ?);`, id, name)
		}
		if err != nil {
			return nil, err
		}
		ids[name] = id
	}
	return ids, tx.Commit()
}
