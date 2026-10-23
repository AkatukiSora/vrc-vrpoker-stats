// Package logevent defines the closed, format-independent input event model
// consumed by the parser.
package logevent

import "time"

type Record struct {
	Timestamp     time.Time
	HasTimestamp  bool
	HandStartHint bool
	Event         Event
}

type Event interface {
	isLogEvent()
}

type Card struct {
	Rank string
	Suit string
}

type Location struct {
	WorldID      string
	InstanceUID  string
	InstanceType string
	Owner        string
	Region       string
}

type Destination struct{ Location Location }
type WorldJoining struct{ Location Location }
type EnteringRoom struct{ Name string }
type PlayerJoined struct{ UserUID, Name string }
type WorldLeaving struct{}
type NewGame struct{}
type LocalSeat struct{ Seat int }
type LocalCards struct{ Cards [2]Card }
type SmallBlind struct{ Seat, Amount int }
type BigBlind struct{ Seat, Amount int }
type MinBet struct{ Amount int }
type CommunityCard struct {
	Card  Card
	Valid bool
	Token string
}
type StreetBoundary struct{}
type Fold struct{ Seat int }
type EndTurn struct{ Seat, Amount int }
type ShowCards struct {
	Seat  int
	Cards [2]Card
}
type FoldToOne struct{}
type PotWinner struct{ Seat, Amount int }
type FoldWinner struct{ Seat, Amount int }

func (Destination) isLogEvent()    {}
func (WorldJoining) isLogEvent()   {}
func (EnteringRoom) isLogEvent()   {}
func (PlayerJoined) isLogEvent()   {}
func (WorldLeaving) isLogEvent()   {}
func (NewGame) isLogEvent()        {}
func (LocalSeat) isLogEvent()      {}
func (LocalCards) isLogEvent()     {}
func (SmallBlind) isLogEvent()     {}
func (BigBlind) isLogEvent()       {}
func (MinBet) isLogEvent()         {}
func (CommunityCard) isLogEvent()  {}
func (StreetBoundary) isLogEvent() {}
func (Fold) isLogEvent()           {}
func (EndTurn) isLogEvent()        {}
func (ShowCards) isLogEvent()      {}
func (FoldToOne) isLogEvent()      {}
func (PotWinner) isLogEvent()      {}
func (FoldWinner) isLogEvent()     {}
