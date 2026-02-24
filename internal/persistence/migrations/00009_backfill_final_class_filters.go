package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/pressly/goose/v3"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

func init() {
	goose.AddMigrationContext(Up00009, Down00009)
}

func Up00009(ctx context.Context, tx *sql.Tx) error {
	if err := seedPocketCategories(ctx, tx); err != nil {
		return err
	}
	if err := seedFinalClasses(ctx, tx); err != nil {
		return err
	}
	if err := backfillHandPlayerFilters(ctx, tx); err != nil {
		return err
	}
	return nil
}

func Down00009(context.Context, *sql.Tx) error {
	return nil
}

func seedPocketCategories(ctx context.Context, tx *sql.Tx) error {
	for _, cat := range stats.AllPocketCategories() {
		code := pocketCategoryCode(cat)
		label := stats.PocketCategoryLabel(cat)
		if code == "" || label == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO pocket_categories(code, label) VALUES(?, ?)`, code, label); err != nil {
			return fmt.Errorf("insert pocket category %s: %w", code, err)
		}
	}
	return nil
}

func seedFinalClasses(ctx context.Context, tx *sql.Tx) error {
	for _, cls := range stats.AllMadeHandClasses() {
		code := finalClassCode(cls)
		if code == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO final_classes(code, label) VALUES(?, ?)`, code, cls); err != nil {
			return fmt.Errorf("insert final class %s: %w", code, err)
		}
	}
	return nil
}

func backfillHandPlayerFilters(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT hand_uid, local_seat FROM hands WHERE local_seat >= 0`)
	if err != nil {
		return fmt.Errorf("select hands for backfill: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var handUID string
		var localSeat int
		if err := rows.Scan(&handUID, &localSeat); err != nil {
			return fmt.Errorf("scan hand row: %w", err)
		}

		holeCards, err := loadHoleCards(ctx, tx, handUID, localSeat)
		if err != nil {
			return err
		}
		if len(holeCards) != 2 {
			continue
		}

		pocketCats := stats.ClassifyPocketHand(holeCards[0], holeCards[1])
		pocketID, err := pickPocketCategoryID(ctx, tx, pocketCats)
		if err != nil {
			return err
		}

		var finalID any = nil
		boardCards, err := loadBoardCards(ctx, tx, handUID)
		if err != nil {
			return err
		}
		if len(boardCards) >= 5 {
			cls := stats.ClassifyMadeHand(holeCards, boardCards)
			id, err := lookupFinalClassID(ctx, tx, cls)
			if err != nil {
				return err
			}
			if id > 0 {
				finalID = id
			}
		}

		if pocketID == 0 && finalID == nil {
			continue
		}

		if _, err := tx.ExecContext(ctx, `UPDATE hand_players
			SET pocket_category_id = ?, final_class_id = ?
			WHERE hand_uid = ? AND seat_id = ?`,
			pocketIDOrNull(pocketID), finalID, handUID, localSeat); err != nil {
			return fmt.Errorf("update hand_players filters: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate hands: %w", err)
	}
	return nil
}

func loadHoleCards(ctx context.Context, tx *sql.Tx, handUID string, seat int) ([]parser.Card, error) {
	rows, err := tx.QueryContext(ctx, `SELECT card_index, rank, suit FROM hand_hole_cards WHERE hand_uid = ? AND seat_id = ?`, handUID, seat)
	if err != nil {
		return nil, fmt.Errorf("select hole cards: %w", err)
	}
	defer rows.Close()

	type holeRow struct {
		idx  int
		rank string
		suit string
	}
	list := make([]holeRow, 0, 2)
	for rows.Next() {
		var r holeRow
		if err := rows.Scan(&r.idx, &r.rank, &r.suit); err != nil {
			return nil, fmt.Errorf("scan hole cards: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows hole cards: %w", err)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].idx < list[j].idx })
	cards := make([]parser.Card, 0, len(list))
	for _, r := range list {
		cards = append(cards, parser.Card{Rank: r.rank, Suit: r.suit})
	}
	return cards, nil
}

func loadBoardCards(ctx context.Context, tx *sql.Tx, handUID string) ([]parser.Card, error) {
	rows, err := tx.QueryContext(ctx, `SELECT card_index, rank, suit FROM hand_board_cards WHERE hand_uid = ?`, handUID)
	if err != nil {
		return nil, fmt.Errorf("select board cards: %w", err)
	}
	defer rows.Close()

	type boardRow struct {
		idx  int
		rank string
		suit string
	}
	list := make([]boardRow, 0, 5)
	for rows.Next() {
		var r boardRow
		if err := rows.Scan(&r.idx, &r.rank, &r.suit); err != nil {
			return nil, fmt.Errorf("scan board cards: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows board cards: %w", err)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].idx < list[j].idx })
	cards := make([]parser.Card, 0, len(list))
	for _, r := range list {
		cards = append(cards, parser.Card{Rank: r.rank, Suit: r.suit})
	}
	return cards, nil
}

func pickPocketCategoryID(ctx context.Context, tx *sql.Tx, cats []stats.PocketCategory) (int, error) {
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

func pocketIDOrNull(id int) any {
	if id <= 0 {
		return nil
	}
	return id
}
