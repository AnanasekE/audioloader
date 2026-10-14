package db

import "audioloader/internal/audio"

type MemDB struct {
	tracks []*audio.Track
}
