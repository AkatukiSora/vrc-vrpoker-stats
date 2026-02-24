// gen_testlog generates synthetic VRChat VR Poker log files for testing.
//
// It reads existing log files to extract real hand blocks (from
// "[Table]: Preparing for New Game" to the next such marker), then
// reassembles them—with optional card/bet mutation—into new files of
// varying sizes.
//
// Usage:
//
//	go run ./tools/gen_testlog [flags]
//
// Flags:
//
//	--input-dir   directory containing real output_log_*.txt files (default: ".")
//	--output-dir  where to write generated files (default: "./testdata/generated")
//	--count       number of files to generate (default: 100)
//	--min-size    minimum file size in bytes (default: 2097152  = 2 MiB)
//	--max-size    maximum file size in bytes (default: 52428800 = 50 MiB)
//	--seed        random seed; 0 = use current time (default: 0)
//	--start-date  base date for generated timestamps, YYYY-MM-DD (default: 2025-01-01)
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Regex patterns (same as parser, kept local to avoid import cycles)
// ─────────────────────────────────────────────────────────────────────────────

var (
	reTimestamp     = regexp.MustCompile(`^(\d{4}\.\d{2}\.\d{2} \d{2}:\d{2}:\d{2}) (\w+)\s+-\s+(.+)$`)
	reNewGame       = regexp.MustCompile(`\[Table\]: Preparing for New Game`)
	reSBBet         = regexp.MustCompile(`(\[Seat\]: Player \d+ SB BET IN = )(\d+)`)
	reBBBet         = regexp.MustCompile(`(\[Seat\]: Player \d+ BB BET IN = )(\d+)`)
	reEndTurn       = regexp.MustCompile(`(\[Seat\]: Player \d+ End Turn with BET IN = )(\d+)`)
	reNewMinBet     = regexp.MustCompile(`(\[Table\]: New Min Bet: )(\d+)( === New Min Raise: )(\d+)`)
	rePotWinner     = regexp.MustCompile(`(\[Pot\]: Winner: \d+ Pot Amount: )(\d+)`)
	rePotManager    = regexp.MustCompile(`(\[PotManager\]: All players folded, player \d+ won )(\d+)`)
	reCommunityCard = regexp.MustCompile(`\[Table\]: New Community Card: (\S+)`)
	reDrawHole      = regexp.MustCompile(`\[Seat\]: Draw Local Hole Cards: (.+)`)
	reShowHole      = regexp.MustCompile(`(\[Seat\]: Player \d+ Show hole cards: )(.+)`)
)

const timeLayout = "2006.01.02 15:04:05"

// ─────────────────────────────────────────────────────────────────────────────
// HandBlock: one complete hand's worth of log messages (no timestamps)
// ─────────────────────────────────────────────────────────────────────────────

// HandBlock stores a single poker hand extracted from a real log file.
type HandBlock struct {
	// messages holds the raw message portion of each log line (everything after
	// "YYYY.MM.DD HH:MM:SS Debug      -  ").
	messages []string
	// relTimes holds the duration from the start of the block for each line.
	relTimes []time.Duration
	// level holds the log level word for each line (Debug/Warning/Error).
	levels []string
}

// ─────────────────────────────────────────────────────────────────────────────
// Extraction
// ─────────────────────────────────────────────────────────────────────────────

// extractHandBlocks reads a VRChat log file and returns all complete hand
// blocks found within it.  A block starts at a "Preparing for New Game" line
// and ends just before the next such line (or EOF, in which case the last hand
// is incomplete and is discarded).
func extractHandBlocks(path string) ([]HandBlock, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var blocks []HandBlock
	var cur *HandBlock
	var blockStart time.Time

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MiB line buffer
	for scanner.Scan() {
		line := scanner.Text()
		m := reTimestamp.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ts, err := time.Parse(timeLayout, m[1])
		if err != nil {
			continue
		}
		level := m[2]
		msg := strings.TrimSpace(m[3])

		if reNewGame.MatchString(msg) {
			// Finalise previous block (it is now complete).
			if cur != nil && len(cur.messages) > 0 {
				blocks = append(blocks, *cur)
			}
			// Start a new block.
			cur = &HandBlock{}
			blockStart = ts
		}

		if cur != nil {
			cur.messages = append(cur.messages, msg)
			cur.relTimes = append(cur.relTimes, ts.Sub(blockStart))
			cur.levels = append(cur.levels, level)
		}
	}
	// The last open block (no trailing "Preparing" marker) is discarded because
	// we cannot know whether it was fully recorded.

	return blocks, scanner.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// Card helpers
// ─────────────────────────────────────────────────────────────────────────────

var (
	allRanks = []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A"}
	allSuits = []string{"h", "d", "c", "s"}
)

// allCards is the full 52-card deck.
var allCards []string

func init() {
	for _, r := range allRanks {
		for _, s := range allSuits {
			allCards = append(allCards, r+s)
		}
	}
}

// parseCards splits a comma-separated card list like "Ac, Kh" into tokens.
func parseCards(s string) []string {
	var out []string
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok != "" {
			out = append(out, tok)
		}
	}
	return out
}

// collectCards returns all card tokens that appear in a hand block.
func collectCards(b HandBlock) map[string]bool {
	used := make(map[string]bool)
	for _, msg := range b.messages {
		if m := reCommunityCard.FindStringSubmatch(msg); m != nil {
			used[m[1]] = true
		}
		if m := reDrawHole.FindStringSubmatch(msg); m != nil {
			for _, c := range parseCards(m[1]) {
				used[c] = true
			}
		}
		if m := reShowHole.FindStringSubmatch(msg); m != nil {
			for _, c := range parseCards(m[2]) {
				used[c] = true
			}
		}
	}
	return used
}

// buildCardMapping picks a bijective replacement for every card currently in
// the hand, drawing from the cards NOT currently in use.
func buildCardMapping(used map[string]bool, rng *rand.Rand) map[string]string {
	// Candidates are all cards not already used.
	var candidates []string
	for _, c := range allCards {
		if !used[c] {
			candidates = append(candidates, c)
		}
	}
	rng.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })

	mapping := make(map[string]string, len(used))
	i := 0
	for orig := range used {
		if i >= len(candidates) {
			// More cards used than replacements available (shouldn't happen with a
			// sane hand, but be defensive: keep the original card).
			mapping[orig] = orig
		} else {
			mapping[orig] = candidates[i]
			i++
		}
	}
	return mapping
}

// applyCardMapping replaces all card tokens in a line message according to
// the provided mapping.  Cards appear in comma-separated lists; we replace
// each token precisely to avoid partial-string matches.
func applyCardMapping(msg string, mapping map[string]string) string {
	// Community card line: "[Table]: New Community Card: Xr"
	if m := reCommunityCard.FindStringSubmatch(msg); m != nil {
		if rep, ok := mapping[m[1]]; ok {
			return strings.Replace(msg, m[1], rep, 1)
		}
		return msg
	}
	// Draw local hole cards: "[Seat]: Draw Local Hole Cards: Xr, Yr"
	if m := reDrawHole.FindStringSubmatch(msg); m != nil {
		return replaceCardList(msg, m[1], mapping)
	}
	// Show hole cards: "[Seat]: Player N Show hole cards: Xr, Yr"
	if m := reShowHole.FindStringSubmatch(msg); m != nil {
		return replaceCardList(msg, m[2], mapping)
	}
	return msg
}

// replaceCardList rebuilds a card list string using the mapping and splices it
// back into the original message.
func replaceCardList(msg, cardList string, mapping map[string]string) string {
	tokens := parseCards(cardList)
	replaced := make([]string, len(tokens))
	for i, tok := range tokens {
		if rep, ok := mapping[tok]; ok {
			replaced[i] = rep
		} else {
			replaced[i] = tok
		}
	}
	newList := strings.Join(replaced, ", ")
	return strings.Replace(msg, cardList, newList, 1)
}

// ─────────────────────────────────────────────────────────────────────────────
// Mutation
// ─────────────────────────────────────────────────────────────────────────────

// mutateCards returns a copy of the hand block with all card tokens replaced by
// a consistent bijective mapping (no card appears twice in the same hand).
func mutateCards(b HandBlock, rng *rand.Rand) HandBlock {
	used := collectCards(b)
	if len(used) == 0 {
		return b
	}
	mapping := buildCardMapping(used, rng)

	out := HandBlock{
		messages: make([]string, len(b.messages)),
		relTimes: b.relTimes, // immutable slices shared by value are fine
		levels:   b.levels,
	}
	for i, msg := range b.messages {
		out.messages[i] = applyCardMapping(msg, mapping)
	}
	return out
}

// mutateBets returns a copy of the hand block with all bet amounts scaled by a
// random factor while preserving the BB/SB relationship and rounding to the
// nearest BB.
func mutateBets(b HandBlock, rng *rand.Rand) HandBlock {
	// Detect BB amount from the first BB BET IN line.
	bb := 0
	for _, msg := range b.messages {
		if m := reBBBet.FindStringSubmatch(msg); m != nil {
			if v, err := strconv.Atoi(m[2]); err == nil {
				bb = v
				break
			}
		}
	}
	if bb == 0 {
		return b // no BB found, leave untouched
	}

	// Scale factor: 0.8 – 2.5, rounded so that new_bb is a positive integer.
	scales := []float64{0.5, 0.75, 1.0, 1.5, 2.0, 2.5, 3.0}
	scale := scales[rng.Intn(len(scales))]
	newBB := int(float64(bb)*scale + 0.5)
	if newBB < 1 {
		newBB = 1
	}
	ratio := float64(newBB) / float64(bb)

	scaleAmt := func(s string) string {
		v, err := strconv.Atoi(s)
		if err != nil || v == 0 {
			return s
		}
		scaled := int(float64(v)*ratio + 0.5)
		if scaled < 0 {
			scaled = 0
		}
		return strconv.Itoa(scaled)
	}

	out := HandBlock{
		messages: make([]string, len(b.messages)),
		relTimes: b.relTimes,
		levels:   b.levels,
	}
	for i, msg := range b.messages {
		msg = reSBBet.ReplaceAllStringFunc(msg, func(s string) string {
			m := reSBBet.FindStringSubmatch(s)
			return m[1] + scaleAmt(m[2])
		})
		msg = reBBBet.ReplaceAllStringFunc(msg, func(s string) string {
			m := reBBBet.FindStringSubmatch(s)
			return m[1] + scaleAmt(m[2])
		})
		msg = reEndTurn.ReplaceAllStringFunc(msg, func(s string) string {
			m := reEndTurn.FindStringSubmatch(s)
			return m[1] + scaleAmt(m[2])
		})
		msg = reNewMinBet.ReplaceAllStringFunc(msg, func(s string) string {
			m := reNewMinBet.FindStringSubmatch(s)
			return m[1] + scaleAmt(m[2]) + m[3] + scaleAmt(m[4])
		})
		msg = rePotWinner.ReplaceAllStringFunc(msg, func(s string) string {
			m := rePotWinner.FindStringSubmatch(s)
			return m[1] + scaleAmt(m[2])
		})
		msg = rePotManager.ReplaceAllStringFunc(msg, func(s string) string {
			m := rePotManager.FindStringSubmatch(s)
			return m[1] + scaleAmt(m[2])
		})
		out.messages[i] = msg
	}
	return out
}

// mutate applies a random combination of card and bet mutation to a block.
// Each mutation is independently applied with 70% probability.
func mutate(b HandBlock, rng *rand.Rand) HandBlock {
	if rng.Float64() < 0.70 {
		b = mutateCards(b, rng)
	}
	if rng.Float64() < 0.70 {
		b = mutateBets(b, rng)
	}
	return b
}

// ─────────────────────────────────────────────────────────────────────────────
// Noise lines (VRChat chatter that the parser ignores)
// ─────────────────────────────────────────────────────────────────────────────

var noiseTemplates = []string{
	"[Behaviour] OnPlayerJoined Player_%04d (usr_%08x-%04x-%04x-%04x-%012x)",
	"[Network] Network stats: sent %d bytes/sec, received %d bytes/sec",
	"[AssetBundleDownloadManager] Using default cache directory.",
	"[Behaviour] Holding recovery payload until all bunches are received.",
	"[Behaviour] OnRoomPropertiesUpdate",
	"[Behaviour] Waiting for Properties for VRCPlayer[Remote] %d %d",
	"Warning Failed to update discord rich presence",
	"[Behaviour] Spent %.6fs waiting to discover master client.",
	"UdonManager.OnSceneLoaded took '%.3f'",
	"[Behaviour] Microphone device changing to 'System Default'",
}

// writeNoise writes n noise lines at successive second intervals starting from
// t, and returns the updated time.
func writeNoise(w *bufio.Writer, rng *rand.Rand, t time.Time, n int) time.Time {
	for i := 0; i < n; i++ {
		tmpl := noiseTemplates[rng.Intn(len(noiseTemplates))]
		var msg string
		switch tmpl {
		case noiseTemplates[0]:
			msg = fmt.Sprintf(tmpl, rng.Intn(9999), rng.Uint32(), rng.Intn(0xffff), rng.Intn(0xffff), rng.Intn(0xffff), rng.Uint32())
		case noiseTemplates[1]:
			msg = fmt.Sprintf(tmpl, rng.Intn(50000), rng.Intn(50000))
		case noiseTemplates[5]:
			msg = fmt.Sprintf(tmpl, rng.Intn(100000000), rng.Intn(10))
		case noiseTemplates[7]:
			msg = fmt.Sprintf(tmpl, rng.Float64()*10)
		case noiseTemplates[8]:
			msg = fmt.Sprintf(tmpl, rng.Float64()*2)
		default:
			msg = tmpl
		}
		level := "Debug"
		if strings.HasPrefix(msg, "Warning") {
			level = "Warning"
			msg = strings.TrimPrefix(msg, "Warning ")
		}
		fmt.Fprintf(w, "%s %-10s-  %s\n", t.Format(timeLayout), level+"   ", msg)
		t = t.Add(time.Duration(rng.Intn(3)+1) * time.Second)
	}
	return t
}

// ─────────────────────────────────────────────────────────────────────────────
// VRChat startup header
// ─────────────────────────────────────────────────────────────────────────────

// writeStartupHeader writes a plausible VRChat startup sequence up to (but not
// including) the VR Poker world join line, and returns the updated time.
func writeStartupHeader(w *bufio.Writer, t time.Time) time.Time {
	lines := []string{
		"Odin Serializer ArchitectureInfo initialization with defaults (all unaligned read/writes disabled).",
		"Odin Serializer detected whitelisted runtime platform WindowsPlayer and memory read test succeeded; enabling all unaligned memory read/writes.",
		"initialized Steam connection",
		"Using server environment: Release, bf0942f7",
		"Launching with args: 2",
		"[DebugUI] Registered 6 debug UI pages: AssetBundleMemory, DrawHMD, Log, PlayerInfo, NetworkObjectInfo, PerformanceGraphs",
		"Loaded HRTF: COMB_KEMAR_v2_OUT.sofa.",
		"[AssetBundleDownloadManager] Using default cache directory.",
		"[AssetBundleDownloadManager] Using default cache size: 30.00GB",
		"[Behaviour] Registering Avatar Interaction",
		"VRC Analytics Initialized",
		"[Behaviour] Entering Room: Home",
		"[Behaviour] Joining wrld_4432ea9b-729c-46e3-8eaf-846aa0a37fdd:0~public~region(jp)",
		"[Behaviour] Entering Room: VRChat Home",
		"[Behaviour] Successfully joined room",
	}
	for _, msg := range lines {
		level := "Debug   "
		fmt.Fprintf(w, "%s %-10s-  %s\n", t.Format(timeLayout), level+"  ", msg)
		t = t.Add(time.Second)
	}
	return t
}

// writePokerWorldJoin writes the lines that bring the player into the VR Poker
// world (including the Local Seat Assigned line so the parser can detect the
// local player) and returns the updated time.
func writePokerWorldJoin(w *bufio.Writer, t time.Time, instanceSuffix string, localSeat int) time.Time {
	pokerWorldID := "wrld_aeba3422-1543-4e6f-bd9d-0f41ddc5c4f8"
	lines := []string{
		fmt.Sprintf("[Behaviour] Joining %s:%s~group(grp_57f4df89-42ef-44ce-a66c-49e9f5846643)~groupAccessType(plus)~region(jp)", pokerWorldID, instanceSuffix),
		"[Behaviour] Entering Room: VR Poker",
		"[Behaviour] Joining or Creating Room: VR Poker",
		"[Behaviour] Successfully joined room",
		"[Behaviour] Spent 0.004882813s entering world.",
		"[Behaviour] Waiting to enter network room.",
		"[Behaviour] Spent 0.144043s waiting to enter room.",
		"[Behaviour] Waiting to discover master client.",
		"[Behaviour] Spawning players",
		"[Behaviour] Finished spawning players.",
		fmt.Sprintf("[Manager]: Local Seat Assigned. ID: %d", localSeat),
	}
	for _, msg := range lines {
		fmt.Fprintf(w, "%s %-10s-  %s\n", t.Format(timeLayout), "Debug      ", msg)
		t = t.Add(time.Second)
	}
	return t
}

// ─────────────────────────────────────────────────────────────────────────────
// Hand block writer
// ─────────────────────────────────────────────────────────────────────────────

// writeHandBlock emits all lines of a hand block into w, assigning each line a
// timestamp starting at t plus the block's relative offsets.  Returns the
// time after the last line.
func writeHandBlock(w *bufio.Writer, b HandBlock, t time.Time) time.Time {
	for i, msg := range b.messages {
		ts := t.Add(b.relTimes[i])
		level := b.levels[i]
		fmt.Fprintf(w, "%s %-10s-  %s\n", ts.Format(timeLayout), level+"   ", msg)
	}
	// Advance past the last line by the duration of the block plus a few seconds.
	if len(b.relTimes) > 0 {
		t = t.Add(b.relTimes[len(b.relTimes)-1] + 5*time.Second)
	} else {
		t = t.Add(5 * time.Second)
	}
	return t
}

// ─────────────────────────────────────────────────────────────────────────────
// File generator
// ─────────────────────────────────────────────────────────────────────────────

// generateFile creates a single synthetic log file at path.  It keeps adding
// hand blocks (interspersed with noise) until the file's byte size reaches
// targetSize.
func generateFile(path string, pool []HandBlock, targetSize int64, baseTime time.Time, rng *rand.Rand) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriterSize(f, 1<<20) // 1 MiB write buffer

	t := baseTime

	// 1. Startup noise.
	t = writeStartupHeader(w, t)
	t = t.Add(5 * time.Minute) // simulate time spent in home world

	// 2. Join VR Poker world.  Assign a random local seat (0–7).
	instanceNum := 10000 + rng.Intn(55000)
	localSeat := rng.Intn(8)
	t = writePokerWorldJoin(w, t, strconv.Itoa(instanceNum), localSeat)

	// 3. Add noise before the first hand.
	noiseCount := 5 + rng.Intn(15)
	t = writeNoise(w, rng, t, noiseCount)

	// 4. Keep emitting hands until we hit the target size.
	for {
		// Pick a random hand from the pool and mutate it.
		idx := rng.Intn(len(pool))
		hand := mutate(pool[idx], rng)

		// Write the hand.
		t = writeHandBlock(w, hand, t)

		// Flush periodically to get an accurate size reading.
		if err := w.Flush(); err != nil {
			return err
		}

		info, err := f.Stat()
		if err != nil {
			return err
		}
		if info.Size() >= targetSize {
			break
		}

		// Optional inter-hand noise (50% of the time, 1–10 lines).
		if rng.Float64() < 0.50 {
			noiseN := 1 + rng.Intn(10)
			t = writeNoise(w, rng, t, noiseN)
		}
	}

	// 5. Leave room.
	fmt.Fprintf(w, "%s %-10s-  [Behaviour] OnLeftRoom\n", t.Format(timeLayout), "Debug      ")
	t = t.Add(time.Second)
	fmt.Fprintf(w, "%s %-10s-  [Behaviour] Entering Room: Home\n", t.Format(timeLayout), "Debug      ")

	return w.Flush()
}

// ─────────────────────────────────────────────────────────────────────────────
// main
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	inputDir := flag.String("input-dir", ".", "directory with real output_log_*.txt files")
	outputDir := flag.String("output-dir", "testdata/generated", "output directory")
	count := flag.Int("count", 100, "number of files to generate")
	minSize := flag.Int64("min-size", 2*1024*1024, "minimum file size in bytes (default 2 MiB)")
	maxSize := flag.Int64("max-size", 50*1024*1024, "maximum file size in bytes (default 50 MiB)")
	seed := flag.Int64("seed", 0, "random seed (0 = use current Unix time)")
	startDate := flag.String("start-date", "2025-01-01", "base date for timestamps, YYYY-MM-DD")
	flag.Parse()

	if *count < 1 {
		fmt.Fprintln(os.Stderr, "error: --count must be >= 1")
		os.Exit(1)
	}
	if *minSize > *maxSize {
		fmt.Fprintln(os.Stderr, "error: --min-size must be <= --max-size")
		os.Exit(1)
	}

	// Initialise RNG.
	actualSeed := *seed
	if actualSeed == 0 {
		actualSeed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(actualSeed))
	fmt.Printf("seed: %d\n", actualSeed)

	// Parse base date.
	baseTime, err := time.Parse("2006-01-02", *startDate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid --start-date %q: %v\n", *startDate, err)
		os.Exit(1)
	}

	// Find all real log files.
	matches, err := filepath.Glob(filepath.Join(*inputDir, "output_log_*.txt"))
	if err != nil || len(matches) == 0 {
		fmt.Fprintf(os.Stderr, "error: no output_log_*.txt files found in %q\n", *inputDir)
		os.Exit(1)
	}

	// Extract hand blocks from every input file.
	fmt.Printf("scanning %d input file(s)...\n", len(matches))
	var pool []HandBlock
	for _, path := range matches {
		blocks, err := extractHandBlocks(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", path, err)
			continue
		}
		pool = append(pool, blocks...)
		fmt.Printf("  %s: %d hands\n", filepath.Base(path), len(blocks))
	}
	if len(pool) == 0 {
		fmt.Fprintln(os.Stderr, "error: no hand blocks extracted from input files")
		os.Exit(1)
	}
	fmt.Printf("hand pool: %d blocks\n", len(pool))

	// Create output directory.
	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot create output dir %q: %v\n", *outputDir, err)
		os.Exit(1)
	}

	// Generate files.
	sizeRange := *maxSize - *minSize
	t := baseTime
	for i := 0; i < *count; i++ {
		var targetSize int64
		if sizeRange == 0 {
			targetSize = *minSize
		} else {
			targetSize = *minSize + rng.Int63n(sizeRange+1)
		}

		// Stagger each file's start time by 30 min – 3 h.
		gap := time.Duration(30+rng.Intn(150)) * time.Minute
		t = t.Add(gap)

		fname := fmt.Sprintf("output_log_%s.txt", t.Format("2006-01-02_15-04-05"))
		outPath := filepath.Join(*outputDir, fname)

		if err := generateFile(outPath, pool, targetSize, t, rng); err != nil {
			fmt.Fprintf(os.Stderr, "error generating %s: %v\n", fname, err)
			os.Exit(1)
		}

		info, _ := os.Stat(outPath)
		fmt.Printf("[%3d/%d] %s  %.1f MiB\n", i+1, *count, fname, float64(info.Size())/1024/1024)
	}

	fmt.Printf("\ndone — %d files written to %s\n", *count, *outputDir)
}
