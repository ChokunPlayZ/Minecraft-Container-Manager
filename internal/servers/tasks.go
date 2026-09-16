package servers

import (
	"time"
)

// TaskProgress represents real-time progress for a long-running server operation
// such as modpack installation or file archiving.
type TaskProgress struct {
	ID         string `json:"id"`
	ServerID   string `json:"server_id"`
	Operation  string `json:"operation"`   // "modpack_install", "archive", "unzip"
	Stage      string `json:"stage"`       // "scanning", "compressing", "downloading_archive", "preparing", "extracting", "downloading_mods", "finalizing", "completed", "failed"
	StageTitle string `json:"stage_title"` // e.g. "Downloading Mods (12/45)", "Compressing Files"
	StageIndex int    `json:"stage_index"` // 1-based index (e.g. 1, 2, 3)
	StageTotal int    `json:"stage_total"` // total stages (e.g. 4)
	Percent    int    `json:"percent"`     // 0 - 100
	Message    string `json:"message"`     // detailed message / current filename
	Current    int64  `json:"current"`     // current items or bytes done
	Total      int64  `json:"total"`       // total items or total bytes
	BytesDone  int64  `json:"bytes_done"`
	BytesTotal int64  `json:"bytes_total"`
	Error      string `json:"error,omitempty"`
	UpdatedAt  string `json:"updated_at"`
}

// SetTaskProgress records active progress for a server and broadcasts to all subscribers.
func (s *Store) SetTaskProgress(serverID string, p TaskProgress) {
	s.tasksMu.Lock()
	p.ServerID = serverID
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if s.tasks == nil {
		s.tasks = make(map[string]*TaskProgress)
	}
	s.tasks[serverID] = &p

	var subs []chan TaskProgress
	if s.taskSubs != nil {
		subs = append(subs, s.taskSubs[serverID]...)
	}
	s.tasksMu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- p:
		default:
		}
	}
}

// ClearTaskProgress clears any recorded task progress for a server.
func (s *Store) ClearTaskProgress(serverID string) {
	s.tasksMu.Lock()
	defer s.tasksMu.Unlock()
	if s.tasks != nil {
		delete(s.tasks, serverID)
	}
}

// GetTaskProgress returns active task progress for a server, or nil.
func (s *Store) GetTaskProgress(serverID string) *TaskProgress {
	s.tasksMu.RLock()
	defer s.tasksMu.RUnlock()
	if s.tasks == nil {
		return nil
	}
	p := s.tasks[serverID]
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

// SubscribeTaskProgress returns a channel receiving progress updates for a server.
func (s *Store) SubscribeTaskProgress(serverID string) (chan TaskProgress, func()) {
	s.tasksMu.Lock()
	ch := make(chan TaskProgress, 20)
	if s.taskSubs == nil {
		s.taskSubs = make(map[string][]chan TaskProgress)
	}
	if s.tasks != nil {
		if p, ok := s.tasks[serverID]; ok && p != nil {
			ch <- *p
		}
	}
	s.taskSubs[serverID] = append(s.taskSubs[serverID], ch)
	s.tasksMu.Unlock()

	cancel := func() {
		s.tasksMu.Lock()
		defer s.tasksMu.Unlock()
		if s.taskSubs == nil {
			return
		}
		subs := s.taskSubs[serverID]
		for i, c := range subs {
			if c == ch {
				s.taskSubs[serverID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}
	return ch, cancel
}
