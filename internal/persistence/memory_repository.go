package persistence

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
)

type inMemoryEntry struct {
	hand   *parser.Hand
	source HandSourceRef
}

type MemoryRepository struct {
	mu      sync.RWMutex
	hands   map[string]inMemoryEntry
	cursors map[string]ImportCursor
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		hands:   make(map[string]inMemoryEntry),
		cursors: make(map[string]ImportCursor),
	}
}

func (r *MemoryRepository) UpsertHands(_ context.Context, hands []PersistedHand) (UpsertResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.upsertHandsLocked(hands), nil
}

func (r *MemoryRepository) upsertHandsLocked(hands []PersistedHand) UpsertResult {
	res := UpsertResult{}
	for _, ph := range hands {
		if ph.Hand == nil {
			res.Skipped++
			continue
		}
		uid := ph.Source.HandUID
		if uid == "" {
			uid = GenerateHandUID(ph.Hand, ph.Source)
		}
		if _, ok := r.hands[uid]; ok {
			res.Updated++
		} else {
			res.Inserted++
		}
		r.hands[uid] = inMemoryEntry{hand: cloneHand(ph.Hand), source: ph.Source}
	}
	return res
}

func (r *MemoryRepository) ListHands(_ context.Context, f HandFilter) ([]*parser.Hand, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*parser.Hand, 0, len(r.hands))
	for uid, entry := range r.hands {
		h := entry.hand
		if h == nil {
			continue
		}
		if f.OnlyComplete && !h.IsComplete {
			continue
		}
		if f.FromTime != nil && h.StartTime.Before(*f.FromTime) {
			continue
		}
		if f.ToTime != nil && h.StartTime.After(*f.ToTime) {
			continue
		}
		if f.LocalSeat != nil {
			if _, ok := h.Players[*f.LocalSeat]; !ok {
				continue
			}
		}
		copyHand := cloneHand(h)
		copyHand.HandUID = uid
		out = append(out, copyHand)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].StartTime.Before(out[j].StartTime)
	})

	return out, nil
}

func (r *MemoryRepository) CountHands(ctx context.Context, f HandFilter) (int, error) {
	hands, err := r.ListHands(ctx, f)
	if err != nil {
		return 0, err
	}
	return len(hands), nil
}

func (r *MemoryRepository) GetCursor(_ context.Context, sourcePath string) (*ImportCursor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.cursors[sourcePath]
	if !ok {
		return nil, nil
	}
	copyCursor := c
	return &copyCursor, nil
}

func (r *MemoryRepository) SaveCursor(_ context.Context, c ImportCursor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now()
	}
	r.cursors[c.SourcePath] = c
	return nil
}

func (r *MemoryRepository) SaveImportBatch(_ context.Context, hands []PersistedHand, c ImportCursor) (UpsertResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	res := r.upsertHandsLocked(hands)
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now()
	}
	r.cursors[c.SourcePath] = c
	return res, nil
}

func cloneHand(h *parser.Hand) *parser.Hand {
	if h == nil {
		return nil
	}
	copyHand := *h
	copyHand.CommunityCards = append([]parser.Card(nil), h.CommunityCards...)
	copyHand.ActiveSeats = append([]int(nil), h.ActiveSeats...)
	copyHand.InstanceUsers = append([]parser.InstanceUser(nil), h.InstanceUsers...)
	copyHand.Anomalies = append([]parser.HandAnomaly(nil), h.Anomalies...)
	copyHand.Players = make(map[int]*parser.PlayerHandInfo, len(h.Players))
	for seat, pi := range h.Players {
		if pi == nil {
			continue
		}
		copyPI := *pi
		copyPI.HoleCards = append([]parser.Card(nil), pi.HoleCards...)
		copyPI.Actions = append([]parser.PlayerAction(nil), pi.Actions...)
		copyHand.Players[seat] = &copyPI
	}
	return &copyHand
}
