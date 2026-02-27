package parser

// WorldContext holds the minimal parser state required to resume log parsing
// from a committed hand boundary. It captures the world/instance context so
// that hands parsed after a restart carry the correct metadata without
// replaying the full log from byte 0.
//
// This type is also used by persistence.ImportCursor.WorldCtx so that the
// same struct definition serves both the parser layer and the storage boundary,
// eliminating dual WorldContext definitions.
//
// hand.ID (handIDCounter) is intentionally excluded: it is an in-memory
// sequence number that is not stored in the hands table. HandUID (SHA-256) is
// the durable identity, so counter continuity across restarts has no effect on
// correctness or the UI.
type WorldContext struct {
	WorldID          string
	WorldDisplayName string
	InstanceUID      string
	InstanceType     InstanceType
	InstanceOwner    string
	InstanceRegion   string
	InPokerWorld     bool
	WorldDetected    bool
}

// Clone returns a copy of the world context. Update this if pointer or slice
// fields are added in the future.
func (wc WorldContext) Clone() WorldContext {
	return wc
}

// WorldContext returns the current world/instance state of the parser.
// Call this after a hand boundary (when no hand is in progress) to capture
// the context for persistence.
func (p *Parser) WorldContext() WorldContext {
	return WorldContext{
		WorldID:          p.currentWorldID,
		WorldDisplayName: p.currentWorldName,
		InstanceUID:      p.currentInstanceUID,
		InstanceType:     p.currentInstanceType,
		InstanceOwner:    p.currentInstanceOwner,
		InstanceRegion:   p.currentInstanceRegion,
		InPokerWorld:     p.inPokerWorld,
		WorldDetected:    p.worldDetected,
	}
}

// RestoreWorldContext reinitialises the parser with previously persisted world
// context. This must be called on a freshly constructed Parser before any lines
// are fed to it. Instance users are NOT restored here — they are re-populated
// as the parser encounters OnPlayerJoined events in the resumed section.
func (p *Parser) RestoreWorldContext(wc WorldContext) {
	p.currentWorldID = wc.WorldID
	p.currentWorldName = wc.WorldDisplayName
	p.currentInstanceUID = wc.InstanceUID
	p.currentInstanceType = wc.InstanceType
	p.currentInstanceOwner = wc.InstanceOwner
	p.currentInstanceRegion = wc.InstanceRegion
	p.inPokerWorld = wc.InPokerWorld
	p.worldDetected = wc.WorldDetected
}
