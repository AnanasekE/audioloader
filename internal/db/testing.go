package db

import "path/filepath"

// UseTemp points DB, TracksDir, CoversDir and ArtistImagesDir at a fresh, migrated database
// in dir, for tests. restore puts the previous values back.
func UseTemp(dir string) (restore func(), err error) {
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		return nil, err
	}
	if err := Migrate(d); err != nil {
		d.Close()
		return nil, err
	}
	oldDB, oldTracks, oldCovers, oldArtists := DB, TracksDir, CoversDir, ArtistImagesDir
	DB, TracksDir, CoversDir, ArtistImagesDir = d, filepath.Join(dir, "tracks"), filepath.Join(dir, "covers"), filepath.Join(dir, "artists")
	return func() {
		d.Close()
		DB, TracksDir, CoversDir, ArtistImagesDir = oldDB, oldTracks, oldCovers, oldArtists
	}, nil
}
