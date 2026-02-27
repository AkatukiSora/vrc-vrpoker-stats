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
		r.hands[uid] = inMemoryEntry{hand: parser.CloneHand(ph.Hand), source: ph.Source}
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
		copyHand := parser.CloneHand(h)
		copyHand.HandUID = uid
		out = append(out, copyHand)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].StartTime.Before(out[j].StartTime)
	})

	return out, nil
}

func (r *MemoryRepository) CountHands(ctx context.Context, f HandFilter) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, entry := range r.hands {
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
		count++
	}
	return count, nil
}

// GetHandByUID returns the full hand for the given UID, or nil if not found.
func (r *MemoryRepository) GetHandByUID(_ context.Context, uid string) (*parser.Hand, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.hands[uid]
	if !ok || entry.hand == nil {
		return nil, nil
	}
	h := parser.CloneHand(entry.hand)
	h.HandUID = uid
	return h, nil
}

func (r *MemoryRepository) ListHandSummaries(_ context.Context, f HandFilter) ([]HandSummary, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]HandSummary, 0, len(r.hands))
	for uid, entry := range r.hands {
		h := entry.hand
		if h == nil || !h.IsComplete {
			continue
		}
		localSeat := h.LocalPlayerSeat
		if localSeat < 0 {
			continue
		}
		if f.FromTime != nil && h.StartTime.Before(*f.FromTime) {
			continue
		}
		if f.ToTime != nil && h.StartTime.After(*f.ToTime) {
			continue
		}
		if _, ok := h.Players[localSeat]; !ok {
			continue
		}

		s := HandSummary{
			HandUID:    uid,
			StartTime:  h.StartTime,
			NumPlayers: h.NumPlayers,
			TotalPot:   h.TotalPot,
			IsComplete: h.IsComplete,
			LocalSeat:  localSeat,
		}

		// Populate local player fields.
		pi := h.Players[localSeat]
		if pi != nil {
			s.PotWon = pi.PotWon
			s.Won = pi.Won
			if pi.Position != 0 {
				s.Position = pi.Position.String()
			}
			if len(pi.HoleCards) == 2 {
				s.HoleCard0 = pi.HoleCards[0].Rank + pi.HoleCards[0].Suit
				s.HoleCard1 = pi.HoleCards[1].Rank + pi.HoleCards[1].Suit
			}
			// Compute NetChips = PotWon - total invested by local player.
			var invested int
			for _, act := range pi.Actions {
				invested += act.Amount
			}
			s.NetChips = s.PotWon - invested
		}

		out = append(out, s)
	}

	// Newest first (DESC by start_time).
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartTime.After(out[j].StartTime)
	})

	fullCount := len(out)
	if f.Limit > 0 {
		start := f.Offset
		if start > len(out) {
			start = len(out)
		}
		end := start + f.Limit
		if end > len(out) {
			end = len(out)
		}
		out = out[start:end]
	}

	return out, fullCount, nil
}

func (r *MemoryRepository) ListHandsAfter(_ context.Context, after time.Time, localSeat int) ([]*parser.Hand, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*parser.Hand, 0, len(r.hands))
	for uid, entry := range r.hands {
		h := entry.hand
		if h == nil || !h.IsComplete {
			continue
		}
		if h.StartTime.IsZero() || !h.StartTime.After(after) {
			continue
		}
		if localSeat >= 0 {
			if _, ok := h.Players[localSeat]; !ok {
				continue
			}
		}
		copyHand := parser.CloneHand(h)
		copyHand.HandUID = uid
		out = append(out, copyHand)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].StartTime.Before(out[j].StartTime)
	})

	return out, nil
}

func (r *MemoryRepository) GetCursor(_ context.Context, sourcePath string) (*ImportCursor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.cursors[sourcePath]
	if !ok {
		return nil, nil
	}
	copyCursor := c
	if c.WorldCtx != nil {
		wc := c.WorldCtx.Clone()
		copyCursor.WorldCtx = &wc
	}
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

func (r *MemoryRepository) MarkFullyImported(_ context.Context, sourcePath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if c, ok := r.cursors[sourcePath]; ok {
		c.IsFullyImported = true
		c.UpdatedAt = time.Now()
		r.cursors[sourcePath] = c
	}
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
