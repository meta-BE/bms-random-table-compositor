package domain

import "time"

// ServerState は HTTP サーバの稼働状態。
type ServerState string

const (
	ServerStateStopped ServerState = "stopped"
	ServerStateRunning ServerState = "running"
	ServerStateError   ServerState = "error"
)

// ServerStatus は HTTP サーバの状態スナップショット。
type ServerStatus struct {
	State     ServerState
	Port      int
	StartedAt *time.Time
	LastError string // ServerStateError 時にメッセージを格納
}
