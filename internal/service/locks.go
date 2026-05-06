package service

import "sync"

type targetLocks struct {
	mu     sync.Mutex
	active map[string]string
}

func newTargetLocks() *targetLocks {
	return &targetLocks{active: map[string]string{}}
}

func (l *targetLocks) TryLock(targetID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.active[targetID]; ok {
		return false
	}
	l.active[targetID] = ""
	return true
}

func (l *targetLocks) SetActiveKey(targetID, key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.active[targetID]; ok {
		l.active[targetID] = key
	}
}

func (l *targetLocks) Unlock(targetID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.active, targetID)
}

func (l *targetLocks) ActiveKey(targetID string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active[targetID]
}
