package parser

import (
	"fmt"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/logevent"
)

func (p *Parser) ParseRecord(record logevent.Record) error {
	if !record.HasTimestamp {
		return nil
	}
	p.lastTimestamp = record.Timestamp

	switch event := record.Event.(type) {
	case logevent.Destination:
		p.setLocation(event.Location)
		return nil
	case logevent.EnteringRoom:
		p.currentWorldName = event.Name
		return nil
	case logevent.PlayerJoined:
		if event.UserUID != "" {
			p.currentInstanceUsers[event.UserUID] = event.Name
		}
		return nil
	case logevent.WorldJoining:
		p.setLocation(event.Location)
		p.worldDetected = true
		p.inPokerWorld = p.currentWorldID == VRPokerWorldID
		p.result.InPokerWorld = p.inPokerWorld
		return nil
	case logevent.WorldLeaving:
		p.worldDetected = true
		if p.inPokerWorld {
			p.finalizeCurrentHand()
		}
		p.inPokerWorld = false
		p.currentWorldID = ""
		p.currentWorldName = ""
		p.currentInstanceUID = ""
		p.currentInstanceType = InstanceTypeUnknown
		p.currentInstanceOwner = ""
		p.currentInstanceRegion = ""
		p.currentInstanceUsers = make(map[string]string)
		p.result.InPokerWorld = false
		p.result.LocalPlayerSeat = -1
		return nil
	}

	if p.worldDetected && !p.inPokerWorld {
		return nil
	}
	return p.reducePokerEvent(record.Timestamp, record.Event)
}

func (p *Parser) reducePokerEvent(ts time.Time, event logevent.Event) error {
	switch event := event.(type) {
	case logevent.NewGame:
		p.finalizeCurrentHand()
		p.startNewHand(ts)
		return nil
	case logevent.LocalSeat:
		p.result.LocalPlayerSeat = event.Seat
		if p.currentHand != nil {
			p.currentHand.LocalPlayerSeat = event.Seat
			if p.pendingLocalCards != nil {
				p.assignLocalCards(event.Seat, p.pendingLocalCards, true)
			}
		}
		return nil
	}

	if p.currentHand == nil {
		return nil
	}

	switch event := event.(type) {
	case logevent.LocalCards:
		cards := cardsFromLogEvent(event.Cards)
		p.pendingLocalCards = cards
		p.pendingLocalSeat = -1

		localSeat := p.result.LocalPlayerSeat
		if p.lastBlindSeat >= 0 {
			// Tentative: last blind poster is likely us. The later seat or show
			// event can correct this assignment.
			p.assignLocalCards(p.lastBlindSeat, cards, false)
		} else if localSeat >= 0 {
			p.assignLocalCards(localSeat, cards, false)
		}
	case logevent.SmallBlind:
		p.currentHand.SBSeat = event.Seat
		p.lastBlindSeat = event.Seat
		p.ensurePlayer(event.Seat)
		pi := p.currentHand.Players[event.Seat]
		pi.Actions = append(pi.Actions, PlayerAction{
			Timestamp: ts, PlayerID: event.Seat,
			Street: StreetPreFlop, Action: ActionBlindSB, Amount: event.Amount,
		})
		p.streetBets[event.Seat] = event.Amount
		if event.Amount > p.streetBetAmount {
			p.streetBetAmount = event.Amount
		}
		p.pfActions = append(p.pfActions, pfAction{event.Seat, ActionBlindSB, event.Amount})
	case logevent.BigBlind:
		p.currentHand.BBSeat = event.Seat
		p.lastBlindSeat = event.Seat
		p.ensurePlayer(event.Seat)
		pi := p.currentHand.Players[event.Seat]
		pi.Actions = append(pi.Actions, PlayerAction{
			Timestamp: ts, PlayerID: event.Seat,
			Street: StreetPreFlop, Action: ActionBlindBB, Amount: event.Amount,
		})
		p.streetBets[event.Seat] = event.Amount
		if event.Amount > p.streetBetAmount {
			p.streetBetAmount = event.Amount
		}
		p.pfActions = append(p.pfActions, pfAction{event.Seat, ActionBlindBB, event.Amount})
	case logevent.MinBet:
		if event.Amount > p.streetBetAmount {
			p.streetBetAmount = event.Amount
		}
	case logevent.CommunityCard:
		if !event.Valid {
			p.currentHand.addAnomaly("BOARD_PARSE_ERROR", "warn", fmt.Sprintf("invalid board token: %s", event.Token))
			return nil
		}
		card := cardFromLogEvent(event.Card)
		if len(p.currentHand.CommunityCards) >= 5 {
			p.currentHand.addAnomaly("BOARD_OVERFLOW", "warn", "board card count exceeded 5")
			return nil
		}
		for _, existing := range p.currentHand.CommunityCards {
			if existing.Rank == card.Rank && existing.Suit == card.Suit {
				p.currentHand.addAnomaly("BOARD_DUPLICATE_CARD", "warn", card.String())
				return nil
			}
		}
		p.currentHand.CommunityCards = append(p.currentHand.CommunityCards, card)
		switch len(p.currentHand.CommunityCards) {
		case 1, 2, 3:
			if p.currentStreet < StreetFlop {
				p.currentStreet = StreetFlop
			}
		case 4:
			p.currentStreet = StreetTurn
		case 5:
			p.currentStreet = StreetRiver
		}
	case logevent.StreetBoundary:
		p.streetBets = make(map[int]int)
		p.streetBetAmount = 0
	case logevent.Fold:
		p.foldedThisHand[event.Seat] = true
		p.ensurePlayer(event.Seat)
		pi := p.currentHand.Players[event.Seat]
		pi.Actions = append(pi.Actions, PlayerAction{
			Timestamp: ts, PlayerID: event.Seat,
			Street: p.currentStreet, Action: ActionFold, Amount: 0,
		})
		if p.currentStreet == StreetPreFlop {
			pi.FoldedPF = true
			p.pfActions = append(p.pfActions, pfAction{event.Seat, ActionFold, 0})
		}
	case logevent.EndTurn:
		if p.foldedThisHand[event.Seat] {
			return nil
		}
		p.ensurePlayer(event.Seat)
		action := p.classifyAction(event.Seat, event.Amount)
		pi := p.currentHand.Players[event.Seat]
		pi.Actions = append(pi.Actions, PlayerAction{
			Timestamp: ts, PlayerID: event.Seat,
			Street: p.currentStreet, Action: action, Amount: event.Amount,
		})
		p.streetBets[event.Seat] = event.Amount
		if event.Amount > p.streetBetAmount {
			p.streetBetAmount = event.Amount
		}
		if p.currentStreet == StreetPreFlop {
			p.pfActions = append(p.pfActions, pfAction{event.Seat, action, event.Amount})
		}
	case logevent.ShowCards:
		cards := cardsFromLogEvent(event.Cards)
		p.ensurePlayer(event.Seat)
		pi := p.currentHand.Players[event.Seat]
		pi.ShowedDown = true
		p.currentStreet = StreetShowdown
		if p.pendingLocalCards != nil && cardsMatch(p.pendingLocalCards, cards) {
			if p.result.LocalPlayerSeat < 0 {
				p.result.LocalPlayerSeat = event.Seat
			}
			p.assignLocalCards(event.Seat, cards, true)
		} else if len(pi.HoleCards) == 0 {
			pi.HoleCards = cards
		}
	case logevent.FoldToOne:
		p.currentStreet = StreetShowdown
	case logevent.PotWinner:
		p.pendingWinners = append(p.pendingWinners, pendingWin{event.Seat, event.Amount})
		if p.currentHand.WinType == "" {
			p.currentHand.WinType = "showdown"
		}
	case logevent.FoldWinner:
		p.pendingWinners = append(p.pendingWinners, pendingWin{event.Seat, event.Amount})
		p.currentHand.WinType = "fold"
	}
	return nil
}

func (p *Parser) setLocation(location logevent.Location) {
	if location.WorldID == "" {
		return
	}
	p.currentWorldID = location.WorldID
	p.currentInstanceUID = location.InstanceUID
	p.currentInstanceType = InstanceType(location.InstanceType)
	p.currentInstanceOwner = location.Owner
	p.currentInstanceRegion = location.Region
}

func cardsFromLogEvent(cards [2]logevent.Card) []Card {
	return []Card{cardFromLogEvent(cards[0]), cardFromLogEvent(cards[1])}
}

func cardFromLogEvent(card logevent.Card) Card {
	return Card{Rank: card.Rank, Suit: card.Suit}
}
