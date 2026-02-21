package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	_ "github.com/mattn/go-sqlite3"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	repo := &SQLiteRepository{db: db}
	if err := repo.initSchema(); err != nil {
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

func (r *SQLiteRepository) initSchema() error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS hands (
			hand_uid TEXT PRIMARY KEY,
			source_path TEXT NOT NULL,
			start_byte INTEGER NOT NULL,
			end_byte INTEGER NOT NULL,
			start_line INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			is_complete INTEGER NOT NULL,
			local_seat INTEGER NOT NULL,
			payload_json BLOB NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_hands_source_span ON hands(source_path, start_byte, end_byte);`,
		`CREATE INDEX IF NOT EXISTS idx_hands_start_time ON hands(start_time);`,
		`CREATE TABLE IF NOT EXISTS import_cursors (
			source_path TEXT PRIMARY KEY,
			next_byte_offset INTEGER NOT NULL,
			next_line_number INTEGER NOT NULL,
			last_event_time TEXT,
			last_hand_uid TEXT,
			parser_state_json BLOB,
			updated_at TEXT NOT NULL
		);`,
	}
	for _, q := range ddl {
		if _, err := r.db.Exec(q); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	}
	return nil
}

func (r *SQLiteRepository) UpsertHands(ctx context.Context, hands []PersistedHand) (UpsertResult, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return UpsertResult{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt := `INSERT INTO hands(
		hand_uid, source_path, start_byte, end_byte, start_line, end_line,
		start_time, end_time, is_complete, local_seat, payload_json, updated_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(hand_uid) DO UPDATE SET
		source_path=excluded.source_path,
		start_byte=excluded.start_byte,
		end_byte=excluded.end_byte,
		start_line=excluded.start_line,
		end_line=excluded.end_line,
		start_time=excluded.start_time,
		end_time=excluded.end_time,
		is_complete=excluded.is_complete,
		local_seat=excluded.local_seat,
		payload_json=excluded.payload_json,
		updated_at=excluded.updated_at`

	res := UpsertResult{}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, ph := range hands {
		if ph.Hand == nil {
			res.Skipped++
			continue
		}
		uid := ph.Source.HandUID
		if uid == "" {
			uid = GenerateHandUID(ph.Hand, ph.Source)
		}
		payload, mErr := json.Marshal(ph.Hand)
		if mErr != nil {
			err = mErr
			return UpsertResult{}, err
		}
		_, eErr := tx.ExecContext(
			ctx,
			stmt,
			uid,
			ph.Source.SourcePath,
			ph.Source.StartByte,
			ph.Source.EndByte,
			ph.Source.StartLine,
			ph.Source.EndLine,
			ph.Hand.StartTime.UTC().Format(time.RFC3339Nano),
			ph.Hand.EndTime.UTC().Format(time.RFC3339Nano),
			boolToInt(ph.Hand.IsComplete),
			ph.Hand.LocalPlayerSeat,
			payload,
			now,
		)
		if eErr != nil {
			err = eErr
			return UpsertResult{}, err
		}
		res.Inserted++
	}

	if err = tx.Commit(); err != nil {
		return UpsertResult{}, err
	}
	return res, nil
}

func (r *SQLiteRepository) ListHands(ctx context.Context, f HandFilter) ([]*parser.Hand, error) {
	query := `SELECT payload_json FROM hands WHERE 1=1`
	args := make([]any, 0, 4)
	if f.OnlyComplete {
		query += ` AND is_complete=1`
	}
	if f.FromTime != nil {
		query += ` AND start_time >= ?`
		args = append(args, f.FromTime.UTC().Format(time.RFC3339Nano))
	}
	if f.ToTime != nil {
		query += ` AND start_time <= ?`
		args = append(args, f.ToTime.UTC().Format(time.RFC3339Nano))
	}
	query += ` ORDER BY start_time ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*parser.Hand, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var h parser.Hand
		if err := json.Unmarshal(payload, &h); err != nil {
			return nil, err
		}
		if f.LocalSeat != nil {
			if _, ok := h.Players[*f.LocalSeat]; !ok {
				continue
			}
		}
		hCopy := h
		out = append(out, &hCopy)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *SQLiteRepository) CountHands(ctx context.Context, f HandFilter) (int, error) {
	hands, err := r.ListHands(ctx, f)
	if err != nil {
		return 0, err
	}
	return len(hands), nil
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
	_, err := r.db.ExecContext(
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

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
