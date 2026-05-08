package domain

import "time"

// SongdataAttachStatus は SongdataAttacher の状態スナップショット (GUI 表示用)。
type SongdataAttachStatus struct {
	Attached   bool
	Path       string
	SongCount  int        // SELECT COUNT(*) FROM sd.song の最終値
	AttachedAt *time.Time
	LastError  string
}
