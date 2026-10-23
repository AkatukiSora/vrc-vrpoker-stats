package logformat

import (
	"reflect"
	"testing"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/logevent"
)

func TestLegacyLexerDecodeEventsInPriorityOrder(t *testing.T) {
	ts := time.Date(2026, time.February, 1, 2, 43, 0, 0, time.UTC)
	location := logevent.Location{
		WorldID:      "wrld_aeba3422-1543-4e6f-bd9d-0f41ddc5c4f8",
		InstanceUID:  "wrld_aeba3422-1543-4e6f-bd9d-0f41ddc5c4f8:1~private(owner)~canRequestInvite~region(us)",
		InstanceType: "invite_plus",
		Owner:        "owner",
		Region:       "us",
	}
	cases := []struct {
		name string
		msg  string
		want logevent.Event
		hint bool
	}{
		{"destination before joining", "[Behaviour] Destination set: " + location.InstanceUID + " [Behaviour] Joining " + location.InstanceUID, logevent.Destination{Location: location}, false},
		{"enter room", "[Behaviour] Entering Room: Poker Room", logevent.EnteringRoom{Name: "Poker Room"}, false},
		{"player joined", "[Behaviour] OnPlayerJoined Alice (usr_1234-abcd)", logevent.PlayerJoined{UserUID: "usr_1234-abcd", Name: "Alice"}, false},
		{"world joining", "[Behaviour] Joining " + location.InstanceUID, logevent.WorldJoining{Location: location}, false},
		{"world leaving", "[Behaviour] OnLeftRoom", logevent.WorldLeaving{}, false},
		{"new game before local seat", "[Table]: Preparing for New Game [Manager]: Local Seat Assigned. ID: 3", logevent.NewGame{}, true},
		{"local seat", "[Manager]: Local Seat Assigned. ID: 3", logevent.LocalSeat{Seat: 3}, false},
		{"local cards", "[Seat]: Draw Local Hole Cards: As, Kd", logevent.LocalCards{Cards: [2]logevent.Card{{Rank: "A", Suit: "s"}, {Rank: "K", Suit: "d"}}}, false},
		{"small blind", "[Seat]: Player 1 SB BET IN = 5", logevent.SmallBlind{Seat: 1, Amount: 5}, false},
		{"big blind", "[Seat]: Player 2 BB BET IN = 10", logevent.BigBlind{Seat: 2, Amount: 10}, false},
		{"minimum bet", "[Table]: New Min Bet: 20", logevent.MinBet{Amount: 20}, false},
		{"community card", "[Table]: New Community Card: 7h", logevent.CommunityCard{Card: logevent.Card{Rank: "7", Suit: "h"}, Valid: true, Token: "7h"}, false},
		{"street boundary", "[Table]: Next phase.True - 1", logevent.StreetBoundary{}, false},
		{"fold", "[Seat]: Player 4 Folded.", logevent.Fold{Seat: 4}, false},
		{"end turn", "[Seat]: Player 4 End Turn with BET IN = 20", logevent.EndTurn{Seat: 4, Amount: 20}, false},
		{"show cards", "[Seat]: Player 4 Show hole cards: Qs, Jh", logevent.ShowCards{Seat: 4, Cards: [2]logevent.Card{{Rank: "Q", Suit: "s"}, {Rank: "J", Suit: "h"}}}, false},
		{"fold to one", "[Table]: Fold to One Condition", logevent.FoldToOne{}, false},
		{"pot winner", "[Pot]: Winner: 4 Pot Amount: 40", logevent.PotWinner{Seat: 4, Amount: 40}, false},
		{"fold winner", "[PotManager]: All players folded, player 4 won 15", logevent.FoldWinner{Seat: 4, Amount: 15}, false},
	}

	lexer := LegacyLexer{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			record := lexer.Decode("2026.02.01 02:43:00 INFO - " + tc.msg)
			if !record.HasTimestamp || !record.Timestamp.Equal(ts) {
				t.Fatalf("timestamp = (%v, %v), want (%v, true)", record.Timestamp, record.HasTimestamp, ts)
			}
			if !reflect.DeepEqual(record.Event, tc.want) {
				t.Fatalf("event = %#v, want %#v", record.Event, tc.want)
			}
			if record.HandStartHint != tc.hint {
				t.Fatalf("HandStartHint = %v, want %v", record.HandStartHint, tc.hint)
			}
		})
	}
}

func TestLegacyLexerTimestampHintAndInvalidCards(t *testing.T) {
	lexer := LegacyLexer{}

	noTimestamp := lexer.Decode("noise [Table]: Preparing for New Game")
	if noTimestamp.HasTimestamp || noTimestamp.Event != nil || !noTimestamp.HandStartHint {
		t.Fatalf("no-timestamp record = %#v", noTimestamp)
	}

	invalidTimestamp := lexer.Decode("2026.02.31 02:43:00 INFO - [Table]: Preparing for New Game")
	if invalidTimestamp.HasTimestamp || invalidTimestamp.Event != nil || !invalidTimestamp.HandStartHint {
		t.Fatalf("invalid-timestamp record = %#v", invalidTimestamp)
	}

	unknown := lexer.Decode("2026.02.01 02:43:00 INFO - unrecognized message")
	if !unknown.HasTimestamp || unknown.Event != nil || unknown.HandStartHint {
		t.Fatalf("unknown record = %#v", unknown)
	}

	badCards := lexer.Decode("2026.02.01 02:43:00 INFO - [Seat]: Draw Local Hole Cards: bad")
	if !badCards.HasTimestamp || badCards.Event != nil {
		t.Fatalf("invalid local cards record = %#v", badCards)
	}

	badBoard := lexer.Decode("2026.02.01 02:43:00 INFO - [Table]: New Community Card: ZZ")
	want := logevent.CommunityCard{Valid: false, Token: "ZZ"}
	if !reflect.DeepEqual(badBoard.Event, want) {
		t.Fatalf("invalid board event = %#v, want %#v", badBoard.Event, want)
	}
}

func TestParseCardAndCards(t *testing.T) {
	card, err := ParseCard(" As ")
	if err != nil || card != (logevent.Card{Rank: "A", Suit: "s"}) {
		t.Fatalf("ParseCard() = (%#v, %v)", card, err)
	}
	cards, err := ParseCards("As, Kd")
	if err != nil || !reflect.DeepEqual(cards, []logevent.Card{{Rank: "A", Suit: "s"}, {Rank: "K", Suit: "d"}}) {
		t.Fatalf("ParseCards() = (%#v, %v)", cards, err)
	}
	if _, err := ParseCard("ZZ"); err == nil {
		t.Fatal("ParseCard(ZZ) unexpectedly succeeded")
	}
}
