package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// WAL mode reduces write latency by avoiding full fsync on every commit.
	// synchronous=NORMAL is safe with WAL and significantly faster than the default FULL.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set sqlite pragmas: %w", err)
	}
	repo := &SQLiteRepository{db: db}
	if err := runMigrations(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repo, nil
}

func (r *SQLiteRepository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *SQLiteRepository) UpsertHands(ctx context.Context, hands []PersistedHand) (UpsertResult, error) {
	var res UpsertResult
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		res, err = r.upsertHandsTx(ctx, tx, hands)
		return err
	})
	if err != nil {
		return UpsertResult{}, err
	}
	return res, nil
}

func (r *SQLiteRepository) upsertHandsTx(ctx context.Context, tx *sql.Tx, hands []PersistedHand) (UpsertResult, error) {
	res := UpsertResult{}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	for _, ph := range hands {
		if ph.Hand == nil {
			res.Skipped++
			continue
		}
		h := ph.Hand
		uid := ph.Source.HandUID
		if resolvedUID, ok, err := findHandUIDBySourceSpanTx(ctx, tx, ph.Source); err != nil {
			return UpsertResult{}, err
		} else if ok {
			uid = resolvedUID
		}
		if uid == "" {
			uid = GenerateHandUID(h, ph.Source)
		}

		exists, err := rowExists(ctx, tx, `SELECT 1 FROM hands WHERE hand_uid = ? LIMIT 1`, uid)
		if err != nil {
			return UpsertResult{}, err
		}

		if err := upsertWorldAndInstanceTx(ctx, tx, h, now); err != nil {
			return UpsertResult{}, err
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO hands(
			hand_uid, start_time, end_time, is_complete, stats_eligible, has_anomaly, local_seat,
			world_id, world_display_name, instance_uid, instance_type, instance_owner_user_uid, instance_region,
			sb_seat, bb_seat, num_players, total_pot, winner_seat, win_type, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hand_uid) DO UPDATE SET
			start_time=excluded.start_time,
			end_time=excluded.end_time,
			is_complete=excluded.is_complete,
			stats_eligible=excluded.stats_eligible,
			has_anomaly=excluded.has_anomaly,
			local_seat=excluded.local_seat,
			world_id=excluded.world_id,
			world_display_name=excluded.world_display_name,
			instance_uid=excluded.instance_uid,
			instance_type=excluded.instance_type,
			instance_owner_user_uid=excluded.instance_owner_user_uid,
			instance_region=excluded.instance_region,
			sb_seat=excluded.sb_seat,
			bb_seat=excluded.bb_seat,
			num_players=excluded.num_players,
			total_pot=excluded.total_pot,
			winner_seat=excluded.winner_seat,
			win_type=excluded.win_type,
			updated_at=excluded.updated_at`,
			uid,
			h.StartTime.UTC().Format(time.RFC3339Nano),
			h.EndTime.UTC().Format(time.RFC3339Nano),
			boolToInt(h.IsComplete),
			boolToInt(h.IsStatsEligible()),
			boolToInt(h.HasDataAnomaly()),
			h.LocalPlayerSeat,
			nullIfEmpty(h.WorldID),
			h.WorldDisplayName,
			nullIfEmpty(h.InstanceUID),
			string(defaultInstanceType(h.InstanceType)),
			nullIfEmpty(h.InstanceOwner),
			nullIfEmpty(h.InstanceRegion),
			h.SBSeat,
			h.BBSeat,
			h.NumPlayers,
			h.TotalPot,
			h.WinnerSeat,
			h.WinType,
			now,
		); err != nil {
			return UpsertResult{}, err
		}

		if err := clearHandChildrenTx(ctx, tx, uid); err != nil {
			return UpsertResult{}, err
		}

		if err := insertHandChildrenTx(ctx, tx, uid, h); err != nil {
			return UpsertResult{}, err
		}

		if err := upsertUsersAndParticipantsTx(ctx, tx, h, now); err != nil {
			return UpsertResult{}, err
		}

		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO hand_occurrences(
			hand_uid, source_path, start_byte, end_byte, start_line, end_line, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?)`,
			uid,
			ph.Source.SourcePath,
			ph.Source.StartByte,
			ph.Source.EndByte,
			ph.Source.StartLine,
			ph.Source.EndLine,
			now,
		); err != nil {
			return UpsertResult{}, err
		}

		if exists {
			res.Updated++
		} else {
			res.Inserted++
		}
	}

	return res, nil
}

func upsertWorldAndInstanceTx(ctx context.Context, tx *sql.Tx, h *parser.Hand, now string) error {
	if h == nil {
		return nil
	}
	if h.WorldID != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO worlds(world_id, display_name, updated_at)
			VALUES(?, ?, ?)
			ON CONFLICT(world_id) DO UPDATE SET
				display_name=excluded.display_name,
				updated_at=excluded.updated_at`,
			h.WorldID,
			h.WorldDisplayName,
			now,
		); err != nil {
			return err
		}
	}
	if h.InstanceUID != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO instances(
			instance_uid, world_id, instance_type, owner_user_uid, region, world_display_name, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(instance_uid) DO UPDATE SET
			world_id=excluded.world_id,
			instance_type=excluded.instance_type,
			owner_user_uid=excluded.owner_user_uid,
			region=excluded.region,
			world_display_name=excluded.world_display_name,
			updated_at=excluded.updated_at`,
			h.InstanceUID,
			nullIfEmpty(h.WorldID),
			string(defaultInstanceType(h.InstanceType)),
			nullIfEmpty(h.InstanceOwner),
			nullIfEmpty(h.InstanceRegion),
			h.WorldDisplayName,
			now,
		); err != nil {
			return err
		}
	}
	return nil
}

func upsertUsersAndParticipantsTx(ctx context.Context, tx *sql.Tx, h *parser.Hand, now string) error {
	if h == nil || h.InstanceUID == "" {
		return nil
	}
	for _, u := range h.InstanceUsers {
		if u.UserUID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO users(user_uid, display_name, updated_at)
			VALUES(?, ?, ?)
			ON CONFLICT(user_uid) DO UPDATE SET
				display_name=excluded.display_name,
				updated_at=excluded.updated_at`,
			u.UserUID,
			u.DisplayName,
			now,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO instance_participants(instance_uid, user_uid, first_seen_at, last_seen_at)
			VALUES(?, ?, ?, ?)
			ON CONFLICT(instance_uid, user_uid) DO UPDATE SET
				last_seen_at=excluded.last_seen_at`,
			h.InstanceUID,
			u.UserUID,
			now,
			now,
		); err != nil {
			return err
		}
	}
	return nil
}

func insertHandChildrenTx(ctx context.Context, tx *sql.Tx, uid string, h *parser.Hand) error {
	for i, c := range h.CommunityCards {
		if _, err := tx.ExecContext(ctx, `INSERT INTO hand_board_cards(hand_uid, card_index, rank, suit) VALUES(?, ?, ?, ?)`, uid, i, c.Rank, c.Suit); err != nil {
			return err
		}
	}

	pocketCategoryID, finalClassID, err := computeHandFilterIDs(ctx, tx, h)
	if err != nil {
		return err
	}

	seats := make([]int, 0, len(h.Players))
	for seat := range h.Players {
		seats = append(seats, seat)
	}
	sort.Ints(seats)

	for _, seat := range seats {
		pi := h.Players[seat]
		if pi == nil {
			continue
		}
		pocketID := sql.NullInt64{}
		finalID := sql.NullInt64{}
		if seat == h.LocalPlayerSeat {
			if pocketCategoryID > 0 {
				pocketID = sql.NullInt64{Int64: int64(pocketCategoryID), Valid: true}
			}
			if finalClassID > 0 {
				finalID = sql.NullInt64{Int64: int64(finalClassID), Valid: true}
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hand_players(
			hand_uid, seat_id, position, showed_down, won, pot_won, vpip, pfr, three_bet, fold_to_3bet, folded_pf,
			pocket_category_id, final_class_id
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uid,
			seat,
			int(pi.Position),
			boolToInt(pi.ShowedDown),
			boolToInt(pi.Won),
			pi.PotWon,
			boolToInt(pi.VPIP),
			boolToInt(pi.PFR),
			boolToInt(pi.ThreeBet),
			boolToInt(pi.FoldTo3Bet),
			boolToInt(pi.FoldedPF),
			pocketID,
			finalID,
		); err != nil {
			return err
		}

		for ci, hc := range pi.HoleCards {
			if _, err := tx.ExecContext(ctx, `INSERT INTO hand_hole_cards(hand_uid, seat_id, card_index, rank, suit) VALUES(?, ?, ?, ?, ?)`, uid, seat, ci, hc.Rank, hc.Suit); err != nil {
				return err
			}
		}

		for ai, act := range pi.Actions {
			if _, err := tx.ExecContext(ctx, `INSERT INTO hand_actions(
				hand_uid, seat_id, action_index, timestamp, street, action, amount
			) VALUES(?, ?, ?, ?, ?, ?, ?)`,
				uid,
				seat,
				ai,
				act.Timestamp.UTC().Format(time.RFC3339Nano),
				int(act.Street),
				int(act.Action),
				act.Amount,
			); err != nil {
				return err
			}
		}
	}

	for i, a := range h.Anomalies {
		if _, err := tx.ExecContext(ctx, `INSERT INTO hand_anomalies(hand_uid, anomaly_index, code, severity, detail) VALUES(?, ?, ?, ?, ?)`, uid, i, a.Code, a.Severity, a.Detail); err != nil {
			return err
		}
	}

	return nil
}

func (r *SQLiteRepository) ListHands(ctx context.Context, f HandFilter) ([]*parser.Hand, error) {
	query := `SELECT hand_uid, start_time, end_time, is_complete, stats_eligible, has_anomaly,
		local_seat, world_id, world_display_name, instance_uid, instance_type, instance_owner_user_uid, instance_region,
		sb_seat, bb_seat, num_players, total_pot, winner_seat, win_type
		FROM hands`
	where, args := buildHandsFilterWhere(f)
	query += where
	query += ` ORDER BY start_time ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// First pass: collect all hand headers; preserve insertion order for later.
	uids := make([]string, 0)
	byUID := make(map[string]*parser.Hand)

	for rows.Next() {
		var uid string
		var startStr, endStr string
		var isComplete, statsEligible, hasAnomaly int
		var localSeat int
		var worldID sql.NullString
		var worldDisplayName string
		var instanceUID sql.NullString
		var instanceType string
		var instanceOwner sql.NullString
		var instanceRegion sql.NullString
		var sbSeat, bbSeat, numPlayers, totalPot, winnerSeat int
		var winType string

		if err := rows.Scan(
			&uid,
			&startStr,
			&endStr,
			&isComplete,
			&statsEligible,
			&hasAnomaly,
			&localSeat,
			&worldID,
			&worldDisplayName,
			&instanceUID,
			&instanceType,
			&instanceOwner,
			&instanceRegion,
			&sbSeat,
			&bbSeat,
			&numPlayers,
			&totalPot,
			&winnerSeat,
			&winType,
		); err != nil {
			return nil, err
		}

		startTime, _ := time.Parse(time.RFC3339Nano, startStr)
		endTime, _ := time.Parse(time.RFC3339Nano, endStr)

		h := &parser.Hand{
			HandUID:          uid,
			StartTime:        startTime,
			EndTime:          endTime,
			IsComplete:       isComplete == 1,
			StatsEligible:    statsEligible == 1,
			HasAnomaly:       hasAnomaly == 1,
			LocalPlayerSeat:  localSeat,
			WorldID:          worldID.String,
			WorldDisplayName: worldDisplayName,
			InstanceUID:      instanceUID.String,
			InstanceType:     parser.InstanceType(instanceType),
			InstanceOwner:    instanceOwner.String,
			InstanceRegion:   instanceRegion.String,
			SBSeat:           sbSeat,
			BBSeat:           bbSeat,
			NumPlayers:       numPlayers,
			TotalPot:         totalPot,
			WinnerSeat:       winnerSeat,
			WinType:          winType,
			Players:          make(map[int]*parser.PlayerHandInfo),
		}
		uids = append(uids, uid)
		byUID[uid] = h
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(uids) == 0 {
		return nil, nil
	}

	// Second pass: batch-load all child tables (5 queries total regardless of N).
	if err := r.loadAllHandChildren(ctx, uids, byUID); err != nil {
		return nil, err
	}

	// Apply optional LocalSeat post-filter and assemble result in original order.
	out := make([]*parser.Hand, 0, len(uids))
	for _, uid := range uids {
		h := byUID[uid]
		if f.LocalSeat != nil {
			if _, ok := h.Players[*f.LocalSeat]; !ok {
				continue
			}
		}
		out = append(out, h)
	}
	return out, nil
}

// GetHandByUID fetches the full hand data for a single hand UID.
// Returns nil, nil if the hand is not found.
func (r *SQLiteRepository) GetHandByUID(ctx context.Context, uid string) (*parser.Hand, error) {
	row := r.db.QueryRowContext(ctx, `SELECT hand_uid, start_time, end_time, is_complete, stats_eligible, has_anomaly,
		local_seat, world_id, world_display_name, instance_uid, instance_type, instance_owner_user_uid, instance_region,
		sb_seat, bb_seat, num_players, total_pot, winner_seat, win_type
		FROM hands WHERE hand_uid = ?`, uid)

	var startStr, endStr string
	var isComplete, statsEligible, hasAnomaly int
	var localSeat int
	var worldID sql.NullString
	var worldDisplayName string
	var instanceUID sql.NullString
	var instanceType string
	var instanceOwner sql.NullString
	var instanceRegion sql.NullString
	var sbSeat, bbSeat, numPlayers, totalPot, winnerSeat int
	var winType string

	err := row.Scan(
		&uid,
		&startStr,
		&endStr,
		&isComplete,
		&statsEligible,
		&hasAnomaly,
		&localSeat,
		&worldID,
		&worldDisplayName,
		&instanceUID,
		&instanceType,
		&instanceOwner,
		&instanceRegion,
		&sbSeat,
		&bbSeat,
		&numPlayers,
		&totalPot,
		&winnerSeat,
		&winType,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	startTime, _ := time.Parse(time.RFC3339Nano, startStr)
	endTime, _ := time.Parse(time.RFC3339Nano, endStr)

	h := &parser.Hand{
		HandUID:          uid,
		StartTime:        startTime,
		EndTime:          endTime,
		IsComplete:       isComplete == 1,
		StatsEligible:    statsEligible == 1,
		HasAnomaly:       hasAnomaly == 1,
		LocalPlayerSeat:  localSeat,
		WorldID:          worldID.String,
		WorldDisplayName: worldDisplayName,
		InstanceUID:      instanceUID.String,
		InstanceType:     parser.InstanceType(instanceType),
		InstanceOwner:    instanceOwner.String,
		InstanceRegion:   instanceRegion.String,
		SBSeat:           sbSeat,
		BBSeat:           bbSeat,
		NumPlayers:       numPlayers,
		TotalPot:         totalPot,
		WinnerSeat:       winnerSeat,
		WinType:          winType,
		Players:          make(map[int]*parser.PlayerHandInfo),
	}

	byUID := map[string]*parser.Hand{uid: h}
	if err := r.loadAllHandChildren(ctx, []string{uid}, byUID); err != nil {
		return nil, err
	}
	return h, nil
}

// inClause builds a SQL "IN (?, ?, ...)" placeholder string and returns the
// UIDs as a []any slice suitable for use as variadic query arguments.
func inClause(uids []string) (string, []any) {
	placeholders := make([]byte, 0, len(uids)*3)
	args := make([]any, len(uids))
	for i, uid := range uids {
		if i > 0 {
			placeholders = append(placeholders, ',', '?')
		} else {
			placeholders = append(placeholders, '?')
		}
		args[i] = uid
	}
	return "(" + string(placeholders) + ")", args
}

// sqliteMaxVars is the default SQLite SQLITE_MAX_VARIABLE_NUMBER limit.
// Keeping batches below this prevents "too many SQL variables" errors.
const sqliteMaxVars = 999

// loadAllHandChildren batch-loads board cards, players, hole cards, actions, and
// anomalies for the given set of hand UIDs using 5 queries (one per child table).
// When the number of UIDs exceeds sqliteMaxVars the work is split into chunks.
func (r *SQLiteRepository) loadAllHandChildren(ctx context.Context, uids []string, byUID map[string]*parser.Hand) error {
	// Process in chunks to stay below SQLite's default variable limit of 999.
	for len(uids) > 0 {
		chunk := uids
		if len(chunk) > sqliteMaxVars {
			chunk = uids[:sqliteMaxVars]
		}
		uids = uids[len(chunk):]
		if err := r.loadHandChildrenChunk(ctx, chunk, byUID); err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteRepository) loadHandChildrenChunk(ctx context.Context, uids []string, byUID map[string]*parser.Hand) error {
	in, args := inClause(uids)

	// Board cards
	boardRows, err := r.db.QueryContext(ctx,
		`SELECT hand_uid, card_index, rank, suit FROM hand_board_cards WHERE hand_uid IN `+in+` ORDER BY hand_uid ASC, card_index ASC`, args...)
	if err != nil {
		return err
	}
	for boardRows.Next() {
		var uid string
		var idx int
		var rank, suit string
		if err := boardRows.Scan(&uid, &idx, &rank, &suit); err != nil {
			boardRows.Close()
			return err
		}
		if h, ok := byUID[uid]; ok {
			h.CommunityCards = append(h.CommunityCards, parser.Card{Rank: rank, Suit: suit})
		}
	}
	boardRows.Close()

	// Players
	playerRows, err := r.db.QueryContext(ctx,
		`SELECT hand_uid, seat_id, position, showed_down, won, pot_won, vpip, pfr, three_bet, fold_to_3bet, folded_pf
		 FROM hand_players WHERE hand_uid IN `+in, args...)
	if err != nil {
		return err
	}
	for playerRows.Next() {
		var uid string
		var seat, pos int
		var showedDown, won, vpip, pfr, threeBet, foldTo3Bet, foldedPF int
		var potWon int
		if err := playerRows.Scan(&uid, &seat, &pos, &showedDown, &won, &potWon, &vpip, &pfr, &threeBet, &foldTo3Bet, &foldedPF); err != nil {
			playerRows.Close()
			return err
		}
		if h, ok := byUID[uid]; ok {
			h.Players[seat] = &parser.PlayerHandInfo{
				SeatID:     seat,
				Position:   parser.Position(pos),
				ShowedDown: showedDown == 1,
				Won:        won == 1,
				PotWon:     potWon,
				VPIP:       vpip == 1,
				PFR:        pfr == 1,
				ThreeBet:   threeBet == 1,
				FoldTo3Bet: foldTo3Bet == 1,
				FoldedPF:   foldedPF == 1,
			}
			h.ActiveSeats = append(h.ActiveSeats, seat)
		}
	}
	playerRows.Close()

	// Hole cards
	holeRows, err := r.db.QueryContext(ctx,
		`SELECT hand_uid, seat_id, card_index, rank, suit FROM hand_hole_cards
		 WHERE hand_uid IN `+in+` ORDER BY hand_uid ASC, seat_id ASC, card_index ASC`, args...)
	if err != nil {
		return err
	}
	for holeRows.Next() {
		var uid string
		var seat, idx int
		var rank, suit string
		if err := holeRows.Scan(&uid, &seat, &idx, &rank, &suit); err != nil {
			holeRows.Close()
			return err
		}
		if h, ok := byUID[uid]; ok {
			if pi, ok := h.Players[seat]; ok {
				pi.HoleCards = append(pi.HoleCards, parser.Card{Rank: rank, Suit: suit})
			}
		}
	}
	holeRows.Close()

	// Actions
	actionRows, err := r.db.QueryContext(ctx,
		`SELECT hand_uid, seat_id, action_index, timestamp, street, action, amount FROM hand_actions
		 WHERE hand_uid IN `+in+` ORDER BY hand_uid ASC, seat_id ASC, action_index ASC`, args...)
	if err != nil {
		return err
	}
	for actionRows.Next() {
		var uid string
		var seat, idx int
		var tsStr string
		var street, action, amount int
		if err := actionRows.Scan(&uid, &seat, &idx, &tsStr, &street, &action, &amount); err != nil {
			actionRows.Close()
			return err
		}
		ts, _ := time.Parse(time.RFC3339Nano, tsStr)
		if h, ok := byUID[uid]; ok {
			if pi, ok := h.Players[seat]; ok {
				pi.Actions = append(pi.Actions, parser.PlayerAction{
					Timestamp: ts,
					PlayerID:  seat,
					Street:    parser.Street(street),
					Action:    parser.ActionType(action),
					Amount:    amount,
				})
			}
		}
	}
	actionRows.Close()

	// Anomalies
	anomRows, err := r.db.QueryContext(ctx,
		`SELECT hand_uid, code, severity, detail FROM hand_anomalies WHERE hand_uid IN `+in+` ORDER BY hand_uid ASC, anomaly_index ASC`, args...)
	if err != nil {
		return err
	}
	for anomRows.Next() {
		var uid, code, severity, detail string
		if err := anomRows.Scan(&uid, &code, &severity, &detail); err != nil {
			anomRows.Close()
			return err
		}
		if h, ok := byUID[uid]; ok {
			h.Anomalies = append(h.Anomalies, parser.HandAnomaly{Code: code, Severity: severity, Detail: detail})
		}
	}
	anomRows.Close()

	return nil
}

func (r *SQLiteRepository) ListHandSummaries(ctx context.Context, f HandFilter) ([]HandSummary, int, error) {
	// Build WHERE clause: always restrict to complete hands, plus optional time range.
	where := " WHERE h.is_complete = 1 AND h.local_seat >= 0"
	args := make([]any, 0, 6)
	if f.FromTime != nil {
		where += ` AND h.start_time >= ?`
		args = append(args, f.FromTime.UTC().Format(time.RFC3339Nano))
	}
	if f.ToTime != nil {
		where += ` AND h.start_time <= ?`
		args = append(args, f.ToTime.UTC().Format(time.RFC3339Nano))
	}
	if len(f.PocketCategoryIDs) > 0 {
		where += " AND hp.pocket_category_id IN (" + strings.TrimRight(strings.Repeat("?,", len(f.PocketCategoryIDs)), ",") + ")"
		for _, id := range f.PocketCategoryIDs {
			args = append(args, id)
		}
	}
	if len(f.FinalClassIDs) > 0 {
		where += " AND hp.final_class_id IN (" + strings.TrimRight(strings.Repeat("?,", len(f.FinalClassIDs)), ",") + ")"
		for _, id := range f.FinalClassIDs {
			args = append(args, id)
		}
	}

	// Lightweight summary query for list view. Only local-player data is joined.
	query := `
SELECT
    h.hand_uid,
    h.start_time,
    h.num_players,
    h.total_pot,
    h.is_complete,
    h.local_seat                                                    AS local_seat,
    COALESCE(hp.position, 0)                                        AS position_int,
    COALESCE(hp.pot_won, 0)                                         AS pot_won,
    COALESCE(hp.pot_won, 0) - COALESCE(ag.invested, 0)             AS net_chips,
    COALESCE(hp.won, 0)                                             AS won,
    COALESCE(hc0.rank || hc0.suit, '')                              AS hole_card_0,
    COALESCE(hc1.rank || hc1.suit, '')                              AS hole_card_1,
    COALESCE(bc.community_cards, '')                               AS community_cards,
    COUNT(*) OVER()                                                 AS total_count
FROM hands h
INNER JOIN hand_players hp
    ON hp.hand_uid = h.hand_uid AND hp.seat_id = h.local_seat
LEFT JOIN (
    SELECT ha.hand_uid, SUM(ha.amount) AS invested
    FROM hand_actions ha
    JOIN hands h2 ON h2.hand_uid = ha.hand_uid
    WHERE ha.seat_id = h2.local_seat
    GROUP BY ha.hand_uid
) ag ON ag.hand_uid = h.hand_uid
LEFT JOIN hand_hole_cards hc0
    ON hc0.hand_uid = h.hand_uid AND hc0.seat_id = hp.seat_id AND hc0.card_index = 0
LEFT JOIN hand_hole_cards hc1
    ON hc1.hand_uid = h.hand_uid AND hc1.seat_id = hp.seat_id AND hc1.card_index = 1
LEFT JOIN (
    SELECT hand_uid,
           GROUP_CONCAT(rank || suit, ' ') AS community_cards
    FROM (SELECT hand_uid, rank, suit FROM hand_board_cards ORDER BY hand_uid ASC, card_index ASC)
    GROUP BY hand_uid
) bc ON bc.hand_uid = h.hand_uid` +
		where + `
ORDER BY h.start_time DESC`

	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("ListHandSummaries query: %w", err)
	}
	defer rows.Close()

	var out []HandSummary
	totalCount := 0
	for rows.Next() {
		var s HandSummary
		var startStr string
		var isComplete, won int
		var positionInt int
		var rowTotal int
		if err := rows.Scan(
			&s.HandUID,
			&startStr,
			&s.NumPlayers,
			&s.TotalPot,
			&isComplete,
			&s.LocalSeat,
			&positionInt,
			&s.PotWon,
			&s.NetChips,
			&won,
			&s.HoleCard0,
			&s.HoleCard1,
			&s.CommunityCards,
			&rowTotal,
		); err != nil {
			return nil, 0, fmt.Errorf("ListHandSummaries scan: %w", err)
		}
		if totalCount == 0 {
			totalCount = rowTotal
		}
		s.StartTime, _ = time.Parse(time.RFC3339Nano, startStr)
		s.IsComplete = isComplete == 1
		s.Won = won == 1
		if positionInt != 0 {
			s.Position = parser.Position(positionInt).String()
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("ListHandSummaries rows: %w", err)
	}
	return out, totalCount, nil
}

func (r *SQLiteRepository) ListHandsAfter(ctx context.Context, after time.Time, localSeat int) ([]*parser.Hand, error) {
	query := `SELECT hand_uid, start_time, end_time, is_complete, stats_eligible, has_anomaly,
		local_seat, world_id, world_display_name, instance_uid, instance_type, instance_owner_user_uid, instance_region,
		sb_seat, bb_seat, num_players, total_pot, winner_seat, win_type
		FROM hands
		WHERE is_complete = 1 AND stats_eligible = 1 AND start_time > ?
		ORDER BY start_time ASC`

	rows, err := r.db.QueryContext(ctx, query, after.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	uids := make([]string, 0)
	byUID := make(map[string]*parser.Hand)

	for rows.Next() {
		var uid string
		var startStr, endStr string
		var isComplete, statsEligible, hasAnomaly int
		var localSeatDB int
		var worldID sql.NullString
		var worldDisplayName string
		var instanceUID sql.NullString
		var instanceType string
		var instanceOwner sql.NullString
		var instanceRegion sql.NullString
		var sbSeat, bbSeat, numPlayers, totalPot, winnerSeat int
		var winType string

		if err := rows.Scan(
			&uid, &startStr, &endStr,
			&isComplete, &statsEligible, &hasAnomaly,
			&localSeatDB,
			&worldID, &worldDisplayName, &instanceUID, &instanceType, &instanceOwner, &instanceRegion,
			&sbSeat, &bbSeat, &numPlayers, &totalPot, &winnerSeat, &winType,
		); err != nil {
			return nil, err
		}

		startTime, _ := time.Parse(time.RFC3339Nano, startStr)
		endTime, _ := time.Parse(time.RFC3339Nano, endStr)

		h := &parser.Hand{
			HandUID:          uid,
			StartTime:        startTime,
			EndTime:          endTime,
			IsComplete:       isComplete == 1,
			StatsEligible:    statsEligible == 1,
			HasAnomaly:       hasAnomaly == 1,
			LocalPlayerSeat:  localSeatDB,
			WorldID:          worldID.String,
			WorldDisplayName: worldDisplayName,
			InstanceUID:      instanceUID.String,
			InstanceType:     parser.InstanceType(instanceType),
			InstanceOwner:    instanceOwner.String,
			InstanceRegion:   instanceRegion.String,
			SBSeat:           sbSeat,
			BBSeat:           bbSeat,
			NumPlayers:       numPlayers,
			TotalPot:         totalPot,
			WinnerSeat:       winnerSeat,
			WinType:          winType,
			Players:          make(map[int]*parser.PlayerHandInfo),
		}
		uids = append(uids, uid)
		byUID[uid] = h
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(uids) == 0 {
		return nil, nil
	}

	if err := r.loadAllHandChildren(ctx, uids, byUID); err != nil {
		return nil, err
	}

	out := make([]*parser.Hand, 0, len(uids))
	for _, uid := range uids {
		h := byUID[uid]
		if localSeat >= 0 {
			if _, ok := h.Players[localSeat]; !ok {
				continue
			}
		}
		out = append(out, h)
	}
	return out, nil
}

func (r *SQLiteRepository) CountHands(ctx context.Context, f HandFilter) (int, error) {
	var query string
	var args []any
	if f.LocalSeat != nil {
		// Join hand_players to filter by local seat without loading all hand children.
		baseWhere, baseArgs := buildHandsFilterWhere(f)
		query = `SELECT COUNT(*) FROM hands` + baseWhere +
			` AND EXISTS (SELECT 1 FROM hand_players WHERE hand_players.hand_uid = hands.hand_uid AND hand_players.seat_id = ?)`
		args = append(baseArgs, *f.LocalSeat)
	} else {
		where, whereArgs := buildHandsFilterWhere(f)
		query = `SELECT COUNT(*) FROM hands` + where
		args = whereArgs
	}

	var count int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *SQLiteRepository) GetCursor(ctx context.Context, sourcePath string) (*ImportCursor, error) {
	q := `SELECT source_path, next_byte_offset, next_line_number, last_event_time, last_hand_uid, parser_state_json,
		is_fully_imported,
		world_id, world_display_name, instance_uid, instance_type, instance_owner, instance_region,
		in_poker_world,
		updated_at
		FROM import_cursors WHERE source_path = ?`
	row := r.db.QueryRowContext(ctx, q, sourcePath)
	var c ImportCursor
	var lastEvent sql.NullString
	var updatedAt string
	var isFullyImported, inPokerWorld int
	var worldID, worldDisplayName, instanceUID, instanceType, instanceOwner, instanceRegion sql.NullString
	if err := row.Scan(
		&c.SourcePath,
		&c.NextByteOffset,
		&c.NextLineNumber,
		&lastEvent,
		&c.LastHandUID,
		&c.ParserStateJSON,
		&isFullyImported,
		&worldID,
		&worldDisplayName,
		&instanceUID,
		&instanceType,
		&instanceOwner,
		&instanceRegion,
		&inPokerWorld,
		&updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	c.IsFullyImported = isFullyImported == 1
	if lastEvent.Valid {
		ts, err := time.Parse(time.RFC3339Nano, lastEvent.String)
		if err == nil {
			c.LastEventTime = &ts
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, updatedAt); err == nil {
		c.UpdatedAt = t
	}
	// Restore world context if any world data was persisted.
	if worldID.Valid || instanceUID.Valid || inPokerWorld == 1 {
		c.WorldCtx = &parser.WorldContext{
			WorldID:          worldID.String,
			WorldDisplayName: worldDisplayName.String,
			InstanceUID:      instanceUID.String,
			InstanceType:     parser.InstanceType(instanceType.String),
			InstanceOwner:    instanceOwner.String,
			InstanceRegion:   instanceRegion.String,
			InPokerWorld:     inPokerWorld == 1,
			WorldDetected:    inPokerWorld == 1, // if we were in-world, world was detected
		}
	}
	return &c, nil
}

func (r *SQLiteRepository) SaveCursor(ctx context.Context, c ImportCursor) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		return saveCursorTx(ctx, tx, c)
	})
}

func (r *SQLiteRepository) MarkFullyImported(ctx context.Context, sourcePath string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE import_cursors SET is_fully_imported=1, updated_at=? WHERE source_path=?`,
		time.Now().UTC().Format(time.RFC3339Nano),
		sourcePath,
	)
	return err
}

func (r *SQLiteRepository) SaveImportBatch(ctx context.Context, hands []PersistedHand, c ImportCursor) (UpsertResult, error) {
	var res UpsertResult
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		res, err = r.upsertHandsTx(ctx, tx, hands)
		if err != nil {
			return err
		}
		return saveCursorTx(ctx, tx, c)
	})
	if err != nil {
		return UpsertResult{}, err
	}
	return res, nil
}

func saveCursorTx(ctx context.Context, tx *sql.Tx, c ImportCursor) error {
	updatedAt := c.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	var lastEvent any = nil
	if c.LastEventTime != nil {
		lastEvent = c.LastEventTime.UTC().Format(time.RFC3339Nano)
	}

	// World context columns — all nullable.
	// hand_id_counter exists in the schema (migration 00006) but is intentionally
	// never written: hand.ID is an in-memory sequence not stored in the hands table,
	// so its continuity across restarts has no effect on correctness.
	var worldID, worldDisplayName, instanceUID, instanceType, instanceOwner, instanceRegion any
	inPokerWorld := 0
	if wc := c.WorldCtx; wc != nil {
		worldID = nullIfEmpty(wc.WorldID)
		worldDisplayName = nullIfEmpty(wc.WorldDisplayName)
		instanceUID = nullIfEmpty(wc.InstanceUID)
		instanceType = nullIfEmpty(string(wc.InstanceType))
		instanceOwner = nullIfEmpty(wc.InstanceOwner)
		instanceRegion = nullIfEmpty(wc.InstanceRegion)
		if wc.InPokerWorld {
			inPokerWorld = 1
		}
	}

	q := `INSERT INTO import_cursors(
		source_path, next_byte_offset, next_line_number, last_event_time, last_hand_uid, parser_state_json,
		is_fully_imported,
		world_id, world_display_name, instance_uid, instance_type, instance_owner, instance_region,
		in_poker_world,
		updated_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(source_path) DO UPDATE SET
		next_byte_offset=excluded.next_byte_offset,
		next_line_number=excluded.next_line_number,
		last_event_time=excluded.last_event_time,
		last_hand_uid=excluded.last_hand_uid,
		parser_state_json=excluded.parser_state_json,
		is_fully_imported=excluded.is_fully_imported,
		world_id=excluded.world_id,
		world_display_name=excluded.world_display_name,
		instance_uid=excluded.instance_uid,
		instance_type=excluded.instance_type,
		instance_owner=excluded.instance_owner,
		instance_region=excluded.instance_region,
		in_poker_world=excluded.in_poker_world,
		updated_at=excluded.updated_at`
	_, err := tx.ExecContext(
		ctx,
		q,
		c.SourcePath,
		c.NextByteOffset,
		c.NextLineNumber,
		lastEvent,
		c.LastHandUID,
		c.ParserStateJSON,
		boolToInt(c.IsFullyImported),
		worldID,
		worldDisplayName,
		instanceUID,
		instanceType,
		instanceOwner,
		instanceRegion,
		inPokerWorld,
		updatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func rowExists(ctx context.Context, tx *sql.Tx, query string, args ...any) (bool, error) {
	var probe int
	err := tx.QueryRowContext(ctx, query, args...).Scan(&probe)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func findHandUIDBySourceSpanTx(ctx context.Context, tx *sql.Tx, src HandSourceRef) (string, bool, error) {
	if src.SourcePath == "" {
		return "", false, nil
	}
	var uid string
	err := tx.QueryRowContext(
		ctx,
		`SELECT hand_uid FROM hand_occurrences WHERE source_path = ? AND start_byte = ? AND end_byte = ? LIMIT 1`,
		src.SourcePath,
		src.StartByte,
		src.EndByte,
	).Scan(&uid)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return uid, true, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func computeHandFilterIDs(ctx context.Context, tx *sql.Tx, h *parser.Hand) (int, int, error) {
	if h == nil || h.LocalPlayerSeat < 0 {
		return 0, 0, nil
	}
	pi, ok := h.Players[h.LocalPlayerSeat]
	if !ok || pi == nil || len(pi.HoleCards) != 2 {
		return 0, 0, nil
	}

	pocketCats := stats.ClassifyPocketHand(pi.HoleCards[0], pi.HoleCards[1])
	pocketID, err := lookupPocketCategoryID(ctx, tx, pocketCats)
	if err != nil {
		return 0, 0, err
	}

	finalID := 0
	if len(h.CommunityCards) >= 5 {
		finalClass := stats.ClassifyMadeHand(pi.HoleCards, h.CommunityCards)
		id, err := lookupFinalClassID(ctx, tx, finalClass)
		if err != nil {
			return 0, 0, err
		}
		finalID = id
	}

	return pocketID, finalID, nil
}

func lookupPocketCategoryID(ctx context.Context, tx *sql.Tx, cats []stats.PocketCategory) (int, error) {
	if len(cats) == 0 {
		return 0, nil
	}
	chosen := choosePocketCategory(cats)
	code := pocketCategoryCode(chosen)
	if code == "" {
		return 0, nil
	}
	var id int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM pocket_categories WHERE code = ?`, code).Scan(&id); err != nil {
		return 0, fmt.Errorf("lookup pocket category id: %w", err)
	}
	return id, nil
}

func lookupFinalClassID(ctx context.Context, tx *sql.Tx, cls string) (int, error) {
	code := finalClassCode(cls)
	if code == "" {
		return 0, nil
	}
	var id int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM final_classes WHERE code = ?`, code).Scan(&id); err != nil {
		return 0, fmt.Errorf("lookup final class id: %w", err)
	}
	return id, nil
}

func pocketCategoryCode(c stats.PocketCategory) string {
	switch c {
	case stats.PocketPremium:
		return "premium"
	case stats.PocketSecondPremium:
		return "second_premium"
	case stats.PocketPair:
		return "pair"
	case stats.PocketSuitedConnector:
		return "suited_connector"
	case stats.PocketSuitedOneGapper:
		return "suited_one_gapper"
	case stats.PocketSuited:
		return "suited"
	case stats.PocketAx:
		return "ax"
	case stats.PocketKx:
		return "kx"
	case stats.PocketBroadwayOffsuit:
		return "broadway_offsuit"
	case stats.PocketConnector:
		return "connector"
	default:
		return ""
	}
}

func choosePocketCategory(cats []stats.PocketCategory) stats.PocketCategory {
	priority := []stats.PocketCategory{
		stats.PocketPremium,
		stats.PocketSecondPremium,
		stats.PocketPair,
		stats.PocketSuitedConnector,
		stats.PocketSuitedOneGapper,
		stats.PocketSuited,
		stats.PocketBroadwayOffsuit,
		stats.PocketConnector,
		stats.PocketAx,
		stats.PocketKx,
	}
	set := make(map[stats.PocketCategory]bool, len(cats))
	for _, c := range cats {
		set[c] = true
	}
	for _, p := range priority {
		if set[p] {
			return p
		}
	}
	return cats[0]
}

func finalClassCode(cls string) string {
	return strings.ToLower(strings.ReplaceAll(cls, " ", "_"))
}

func clearHandChildrenTx(ctx context.Context, tx *sql.Tx, handUID string) error {
	tables := []string{"hand_actions", "hand_hole_cards", "hand_players", "hand_board_cards", "hand_anomalies"}
	for _, table := range tables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE hand_uid = ?`, table), handUID); err != nil {
			return err
		}
	}
	return nil
}

func buildHandsFilterWhere(f HandFilter) (string, []any) {
	where := " WHERE 1=1"
	args := make([]any, 0, 3)
	if f.OnlyComplete {
		where += ` AND is_complete=1`
	}
	if f.FromTime != nil {
		where += ` AND start_time >= ?`
		args = append(args, f.FromTime.UTC().Format(time.RFC3339Nano))
	}
	if f.ToTime != nil {
		where += ` AND start_time <= ?`
		args = append(args, f.ToTime.UTC().Format(time.RFC3339Nano))
	}
	return where, args
}

func (r *SQLiteRepository) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func defaultInstanceType(t parser.InstanceType) parser.InstanceType {
	if t == "" {
		return parser.InstanceTypeUnknown
	}
	return t
}
