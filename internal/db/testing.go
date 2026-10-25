package db

import "path/filepath"

// UseTemp points DB, TracksDir and CoversDir at a fresh, migrated database
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
	oldDB, oldTracks, oldCovers := DB, TracksDir, CoversDir
	DB, TracksDir, CoversDir = d, filepath.Join(dir, "tracks"), filepath.Join(dir, "covers")
	return func() {
		d.Close()
		DB, TracksDir, CoversDir = oldDB, oldTracks, oldCovers
	}, nil
}
