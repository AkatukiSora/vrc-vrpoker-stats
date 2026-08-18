# Hand-history export

The application exports one selected hand as [Poker Hand History (PHH)](https://github.com/uoftcprg/phh-std), a TOML-based open interchange format. The compatibility target is tools that implement PHH (including PokerKit-based workflows) and custom analysis scripts. This is deliberately not an imitation PokerStars history: the VRChat log has no verified player screen names, stacks, currency, table identifier, or poker-room identifier, all of which site-specific importers commonly need.

`internal/handhistory` owns PHH serialization. The UI only requests bytes through `application.AppService` and lets the user select the destination; it contains no poker conversion rules.

## Mapping and safety

- The game is emitted as no-limit Texas hold'em (`NT`), with detected SB and BB amounts.
- Players are ordered clockwise from the small blind and named `Seat N`; VRChat account and instance metadata is never exported.
- Hole cards are emitted only when parsed. Unknown cards are `??`, and unknown starting stacks are PHH `inf`.
- Actions are sorted by their parser timestamps within each street. Blinds are represented by PHH's blind field; board cards are inserted at the appropriate street boundary.
- The parser stores timestamps with second precision, so actions sharing exactly the same timestamp have a deterministic seat-number tie-breaker. The source log does not retain a stronger cross-player ordering signal after persistence.
- Exports require at least two detected players plus valid SB and BB posts. Invalid hands fail with an error instead of generating a file that claims unsupported information. PHH itself permits a hand to end before a terminal state, so otherwise valid partial hands can still be exported.
