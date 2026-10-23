// Package logformat owns recognition of the legacy VRChat poker log format.
package logformat

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/logevent"
)

var (
	reTimestamp = regexp.MustCompile(`^(\d{4}\.\d{2}\.\d{2} \d{2}:\d{2}:\d{2}) \w+\s+-\s+(.+)$`)

	reWorldJoining   = regexp.MustCompile(`(?:\[Behaviour\] Joining|Joining friend:) (wrld_[a-f0-9-]+:[^\s]+|wrld_[a-f0-9-]+)`)
	reDestination    = regexp.MustCompile(`\[Behaviour\] Destination (?:requested|fetching|set): (wrld_[a-f0-9-]+:[^\s]+|wrld_[a-f0-9-]+)`)
	reEnteringRoom   = regexp.MustCompile(`\[Behaviour\] Entering Room: (.+)`)
	reOnPlayerJoined = regexp.MustCompile(`\[Behaviour\] OnPlayerJoined (.+?) \((usr_[a-f0-9-]+)\)`)
	reWorldLeaving   = regexp.MustCompile(`\[Behaviour\] OnLeftRoom`)

	reNewGame        = regexp.MustCompile(`\[Table\]: Preparing for New Game`)
	reNewCommunity   = regexp.MustCompile(`\[Table\]: New Community Card: (.+)`)
	reFoldToOne      = regexp.MustCompile(`\[Table\]: Fold to One Condition`)
	reNextPhase      = regexp.MustCompile(`\[Table\]: Next phase\.True - (\d+)`)
	reCollectingBets = regexp.MustCompile(`\[Table\]: Collecting Bets`)
	reNewMinBet      = regexp.MustCompile(`\[Table\]: New Min Bet: (\d+)`)

	reDrawLocalHole = regexp.MustCompile(`\[Seat\]: Draw Local Hole Cards: (.+)`)
	reSBBet         = regexp.MustCompile(`\[Seat\]: Player (\d+) SB BET IN = (\d+)`)
	reBBBet         = regexp.MustCompile(`\[Seat\]: Player (\d+) BB BET IN = (\d+)`)
	rePlayerFolded  = regexp.MustCompile(`\[Seat\]: Player (\d+) Folded\.`)
	rePlayerEndTurn = regexp.MustCompile(`\[Seat\]: Player (\d+) End Turn with BET IN = (\d+)`)
	reShowHoleCards = regexp.MustCompile(`\[Seat\]: Player (\d+) Show hole cards: (.+)`)
	reLocalSeatID   = regexp.MustCompile(`\[Manager\]: Local Seat Assigned\. ID: (\d+)`)

	rePotWinner  = regexp.MustCompile(`\[Pot\]: Winner: (\d+) Pot Amount: (\d+)`)
	rePotManager = regexp.MustCompile(`\[PotManager\]: All players folded, player (\d+) won (\d+)`)
)

var validCardSuits = map[string]bool{"h": true, "d": true, "c": true, "s": true}
var validCardRanks = map[string]bool{
	"2": true, "3": true, "4": true, "5": true, "6": true,
	"7": true, "8": true, "9": true, "10": true,
	"J": true, "Q": true, "K": true, "A": true,
}

const timeLayout = "2006.01.02 15:04:05"

type Lexer interface {
	Decode(line string) logevent.Record
}

type LegacyLexer struct{}

func (LegacyLexer) Decode(line string) logevent.Record {
	record := logevent.Record{HandStartHint: reNewGame.MatchString(line)}
	m := reTimestamp.FindStringSubmatch(line)
	if m == nil {
		return record
	}
	ts, err := time.Parse(timeLayout, m[1])
	if err != nil {
		return record
	}
	record.Timestamp = ts
	record.HasTimestamp = true
	msg := strings.TrimSpace(m[2])

	if dm := reDestination.FindStringSubmatch(msg); dm != nil {
		record.Event = logevent.Destination{Location: parseLocation(dm[1])}
		return record
	}
	if em := reEnteringRoom.FindStringSubmatch(msg); em != nil {
		record.Event = logevent.EnteringRoom{Name: strings.TrimSpace(em[1])}
		return record
	}
	if pm := reOnPlayerJoined.FindStringSubmatch(msg); pm != nil {
		record.Event = logevent.PlayerJoined{UserUID: pm[2], Name: strings.TrimSpace(pm[1])}
		return record
	}
	if wm := reWorldJoining.FindStringSubmatch(msg); wm != nil {
		record.Event = logevent.WorldJoining{Location: parseLocation(wm[1])}
		return record
	}
	if reWorldLeaving.MatchString(msg) {
		record.Event = logevent.WorldLeaving{}
		return record
	}

	// Preserve the legacy poker recognition order. The parser applies the
	// current-hand and world gates after this one-event decode.
	if reNewGame.MatchString(msg) {
		record.Event = logevent.NewGame{}
		return record
	}
	if m := reLocalSeatID.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		record.Event = logevent.LocalSeat{Seat: seat}
		return record
	}
	if m := reDrawLocalHole.FindStringSubmatch(msg); m != nil {
		cards, err := ParseCards(m[1])
		if err == nil && len(cards) == 2 {
			record.Event = logevent.LocalCards{Cards: [2]logevent.Card{cards[0], cards[1]}}
		}
		return record
	}
	if m := reSBBet.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])
		record.Event = logevent.SmallBlind{Seat: seat, Amount: amount}
		return record
	}
	if m := reBBBet.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])
		record.Event = logevent.BigBlind{Seat: seat, Amount: amount}
		return record
	}
	if m := reNewMinBet.FindStringSubmatch(msg); m != nil {
		amount, _ := strconv.Atoi(m[1])
		record.Event = logevent.MinBet{Amount: amount}
		return record
	}
	if m := reNewCommunity.FindStringSubmatch(msg); m != nil {
		token := strings.TrimSpace(m[1])
		card, err := ParseCard(token)
		record.Event = logevent.CommunityCard{Card: card, Valid: err == nil, Token: token}
		return record
	}
	if reNextPhase.MatchString(msg) || reCollectingBets.MatchString(msg) {
		record.Event = logevent.StreetBoundary{}
		return record
	}
	if m := rePlayerFolded.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		record.Event = logevent.Fold{Seat: seat}
		return record
	}
	if m := rePlayerEndTurn.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])
		record.Event = logevent.EndTurn{Seat: seat, Amount: amount}
		return record
	}
	if m := reShowHoleCards.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		cards, err := ParseCards(m[2])
		if err == nil && len(cards) == 2 {
			record.Event = logevent.ShowCards{Seat: seat, Cards: [2]logevent.Card{cards[0], cards[1]}}
		}
		return record
	}
	if reFoldToOne.MatchString(msg) {
		record.Event = logevent.FoldToOne{}
		return record
	}
	if m := rePotWinner.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])
		record.Event = logevent.PotWinner{Seat: seat, Amount: amount}
		return record
	}
	if m := rePotManager.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])
		record.Event = logevent.FoldWinner{Seat: seat, Amount: amount}
	}
	return record
}

func ParseCard(token string) (logevent.Card, error) {
	token = strings.TrimSpace(token)
	if len(token) < 2 {
		return logevent.Card{}, fmtInvalidCard(token)
	}
	suit := string(token[len(token)-1])
	rank := token[:len(token)-1]
	if !validCardSuits[suit] || !validCardRanks[rank] {
		return logevent.Card{}, fmtInvalidCard(token)
	}
	return logevent.Card{Rank: rank, Suit: suit}, nil
}

func ParseCards(tokens string) ([]logevent.Card, error) {
	parts := strings.Split(tokens, ",")
	cards := make([]logevent.Card, 0, len(parts))
	for _, part := range parts {
		card, err := ParseCard(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, nil
}

func parseLocation(raw string) logevent.Location {
	location := strings.TrimSpace(raw)
	if location == "" {
		return logevent.Location{}
	}
	base := location
	tagsPart := ""
	if idx := strings.Index(location, "~"); idx >= 0 {
		base = location[:idx]
		tagsPart = location[idx+1:]
	}
	parts := strings.SplitN(base, ":", 2)
	worldID := parts[0]
	if worldID == "" {
		return logevent.Location{}
	}
	tags := parseLocationTags(tagsPart)
	instanceType := "public"
	owner := ""
	switch {
	case tags["friends"] != "":
		instanceType = "friends"
		owner = tags["friends"]
	case tags["hidden"] != "":
		instanceType = "friends_plus"
		owner = tags["hidden"]
	case tags["private"] != "" && tags["canRequestInvite"] != "":
		instanceType = "invite_plus"
		owner = tags["private"]
	case tags["private"] != "":
		instanceType = "invite"
		owner = tags["private"]
	case tags["group"] != "":
		switch strings.ToLower(tags["groupAccessType"]) {
		case "plus":
			instanceType = "group_plus"
		case "public":
			instanceType = "group_public"
		default:
			instanceType = "group"
		}
	}
	return logevent.Location{
		WorldID:      worldID,
		InstanceUID:  location,
		InstanceType: instanceType,
		Owner:        owner,
		Region:       tags["region"],
	}
}

func parseLocationTags(tagsPart string) map[string]string {
	out := make(map[string]string)
	if tagsPart == "" {
		return out
	}
	for _, token := range strings.Split(tagsPart, "~") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if open := strings.Index(token, "("); open >= 0 && strings.HasSuffix(token, ")") {
			key := token[:open]
			val := token[open+1 : len(token)-1]
			out[key] = val
			continue
		}
		out[token] = "1"
	}
	return out
}

func fmtInvalidCard(token string) error {
	return fmt.Errorf("invalid card token: %q", token)
}
