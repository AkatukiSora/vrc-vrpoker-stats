package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
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
		if _, err := tx.ExecContext(ctx, `INSERT INTO hand_players(
			hand_uid, seat_id, position, showed_down, won, pot_won, vpip, pfr, three_bet, fold_to_3bet, folded_pf
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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

// loadAllHandChildren batch-loads board cards, players, hole cards, actions, and
// anomalies for the given set of hand UIDs using 5 queries (one per child table).
// Results are distributed back into the byUID map.
func (r *SQLiteRepository) loadAllHandChildren(ctx context.Context, uids []string, byUID map[string]*parser.Hand) error {
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
	q := `SELECT source_path, next_byte_offset, next_line_number, last_event_time, last_hand_uid, parser_state_json, updated_at
	FROM import_cursors WHERE source_path = ?`
	row := r.db.QueryRowContext(ctx, q, sourcePath)
	var c ImportCursor
	var lastEvent sql.NullString
	var updated string
	if err := row.Scan(
		&c.SourcePath,
		&c.NextByteOffset,
		&c.NextLineNumber,
		&lastEvent,
		&c.LastHandUID,
		&c.ParserStateJSON,
		&updated,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if lastEvent.Valid {
		ts, err := time.Parse(time.RFC3339Nano, lastEvent.String)
		if err == nil {
			c.LastEventTime = &ts
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, updated); err == nil {
		c.UpdatedAt = t
	}
	return &c, nil
}

func (r *SQLiteRepository) SaveCursor(ctx context.Context, c ImportCursor) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		return saveCursorTx(ctx, tx, c)
	})
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
	q := `INSERT INTO import_cursors(
		source_path, next_byte_offset, next_line_number, last_event_time, last_hand_uid, parser_state_json, updated_at
	) VALUES(?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(source_path) DO UPDATE SET
		next_byte_offset=excluded.next_byte_offset,
		next_line_number=excluded.next_line_number,
		last_event_time=excluded.last_event_time,
		last_hand_uid=excluded.last_hand_uid,
		parser_state_json=excluded.parser_state_json,
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
