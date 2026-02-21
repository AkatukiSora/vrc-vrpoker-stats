package stats

type MetricID string

const (
	MetricVPIP            MetricID = "vpip"
	MetricPFR             MetricID = "pfr"
	MetricGap             MetricID = "gap"
	MetricRFI             MetricID = "rfi"
	MetricColdCall        MetricID = "cold_call"
	MetricThreeBet        MetricID = "three_bet"
	MetricFoldToThreeBet  MetricID = "fold_to_three_bet"
	MetricSteal           MetricID = "steal"
	MetricFoldToSteal     MetricID = "fold_to_steal"
	MetricFlopCBet        MetricID = "flop_cbet"
	MetricTurnCBet        MetricID = "turn_cbet"
	MetricRiverCBet       MetricID = "river_cbet"
	MetricFoldToFlopCBet  MetricID = "fold_to_flop_cbet"
	MetricFoldToTurnCBet  MetricID = "fold_to_turn_cbet"
	MetricFoldToRiverCBet MetricID = "fold_to_river_cbet"
	MetricWTSD            MetricID = "wtsd"
	MetricWSD             MetricID = "w_sd"
	MetricWWSF            MetricID = "wwsf"
	MetricAFq             MetricID = "afq"
	MetricAF              MetricID = "af"
	MetricCheckRaise      MetricID = "check_raise"
	MetricDelayedCBet     MetricID = "delayed_cbet"
	MetricWonWithoutSD    MetricID = "won_without_showdown"
	MetricWonAtSD         MetricID = "won_at_showdown"
	MetricBBPer100        MetricID = "bb_per_100"
)

type MetricSampleClass int

const (
	SampleClassHands MetricSampleClass = iota
	SampleClassSituational
)

type MetricFormat int

const (
	MetricFormatPercent MetricFormat = iota
	MetricFormatRatio
	MetricFormatBBPer100
	MetricFormatDiff
)

type MetricDefinition struct {
	ID          MetricID
	Label       string
	SampleClass MetricSampleClass
	Format      MetricFormat
}

type MetricValue struct {
	ID          MetricID
	Count       int
	Opportunity int
	Rate        float64
	Confident   bool
	MinSample   int
	Format      MetricFormat
}

const (
	handFrequencyThreshold = 200
	situationalThreshold   = 50
)

var metricRegistry = []MetricDefinition{
	{ID: MetricVPIP, Label: "VPIP", SampleClass: SampleClassHands, Format: MetricFormatPercent},
	{ID: MetricPFR, Label: "PFR", SampleClass: SampleClassHands, Format: MetricFormatPercent},
	{ID: MetricGap, Label: "Gap", SampleClass: SampleClassHands, Format: MetricFormatDiff},
	{ID: MetricRFI, Label: "RFI", SampleClass: SampleClassHands, Format: MetricFormatPercent},
	{ID: MetricColdCall, Label: "Cold Call", SampleClass: SampleClassHands, Format: MetricFormatPercent},
	{ID: MetricThreeBet, Label: "3Bet", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricFoldToThreeBet, Label: "Fold to 3Bet", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricSteal, Label: "Steal", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricFoldToSteal, Label: "Fold to Steal", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricFlopCBet, Label: "Flop CBet", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricTurnCBet, Label: "Turn CBet", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricRiverCBet, Label: "River CBet", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricFoldToFlopCBet, Label: "Fold to Flop CBet", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricFoldToTurnCBet, Label: "Fold to Turn CBet", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricFoldToRiverCBet, Label: "Fold to River CBet", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricWTSD, Label: "WTSD", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricWSD, Label: "W$SD", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricWWSF, Label: "WWSF", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricAFq, Label: "AFq", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricAF, Label: "AF", SampleClass: SampleClassSituational, Format: MetricFormatRatio},
	{ID: MetricCheckRaise, Label: "Check-Raise", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricDelayedCBet, Label: "Delayed CBet", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricWonWithoutSD, Label: "Won w/o SD", SampleClass: SampleClassHands, Format: MetricFormatPercent},
	{ID: MetricWonAtSD, Label: "Won at SD", SampleClass: SampleClassSituational, Format: MetricFormatPercent},
	{ID: MetricBBPer100, Label: "bb/100", SampleClass: SampleClassHands, Format: MetricFormatBBPer100},
}

func confidenceThreshold(class MetricSampleClass) int {
	if class == SampleClassSituational {
		return situationalThreshold
	}
	return handFrequencyThreshold
}
