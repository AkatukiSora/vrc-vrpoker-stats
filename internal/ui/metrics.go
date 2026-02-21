package ui

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

type MetricValue struct {
	Display       string
	Color         color.Color
	Opportunities int
}

type MetricDefinition struct {
	ID             string
	Label          string
	Help           string
	MinSamples     int
	ShowInOverview bool
	ShowInPosition bool
	StatsMetricID  *stats.MetricID
	OverviewValue  func(*stats.Stats) MetricValue
	PositionValue  func(*stats.PositionStats) MetricValue
}

type MetricPreset struct {
	Name       string
	MetricIDs  map[string]struct{}
	ButtonText string
}

type MetricVisibilityState struct {
	Visible map[string]bool
}

func NewMetricVisibilityState() *MetricVisibilityState {
	visible := make(map[string]bool, len(metricRegistry))
	for _, m := range metricRegistry {
		visible[m.ID] = true
	}
	return &MetricVisibilityState{Visible: visible}
}

func (m *MetricVisibilityState) IsVisible(metricID string) bool {
	if m == nil {
		return true
	}
	v, ok := m.Visible[metricID]
	if !ok {
		return true
	}
	return v
}

func (m *MetricVisibilityState) SetVisible(metricID string, visible bool) {
	if m == nil {
		return
	}
	if m.Visible == nil {
		m.Visible = make(map[string]bool)
	}
	m.Visible[metricID] = visible
}

func (m *MetricVisibilityState) ApplyPreset(p MetricPreset) {
	if m == nil {
		return
	}
	if m.Visible == nil {
		m.Visible = make(map[string]bool)
	}
	for _, metric := range metricRegistry {
		_, keep := p.MetricIDs[metric.ID]
		m.Visible[metric.ID] = keep
	}
}

func metricsForOverview(visibility *MetricVisibilityState) []MetricDefinition {
	out := make([]MetricDefinition, 0, len(metricRegistry))
	for _, m := range metricRegistry {
		if !m.ShowInOverview {
			continue
		}
		if visibility != nil && !visibility.IsVisible(m.ID) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func metricsForPosition(visibility *MetricVisibilityState) []MetricDefinition {
	out := make([]MetricDefinition, 0, len(metricRegistry))
	for _, m := range metricRegistry {
		if !m.ShowInPosition {
			continue
		}
		if visibility != nil && !visibility.IsVisible(m.ID) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func metricFootnoteText(opportunities, minSamples int) string {
	if opportunities < 0 {
		return ""
	}
	return lang.X("metric.footnote.normal", "n={{.N}}", map[string]any{"N": opportunities})
}

func metricPresets() []MetricPreset {
	beginner := setOf(
		"hands", "winRate", "vpip", "pfr", "gap", "rfi", "three_bet", "fold_to_three_bet",
		"steal", "fold_to_steal", "flop_cbet", "fold_to_flop_cbet", "wtsd", "w_sd", "wwsf", "profit",
	)
	advanced := setOf(
		"hands", "winRate", "vpip", "pfr", "gap", "rfi", "cold_call", "three_bet", "fold_to_three_bet",
		"flop_cbet", "turn_cbet", "river_cbet", "delayed_cbet", "fold_to_flop_cbet", "fold_to_turn_cbet", "fold_to_river_cbet",
		"afq", "af", "check_raise", "wtsd", "w_sd", "wwsf", "won_without_showdown", "won_at_showdown", "bb_per_100", "profit",
	)
	leak := setOf(
		"vpip", "pfr", "gap", "cold_call", "three_bet", "fold_to_three_bet", "steal", "fold_to_steal",
		"flop_cbet", "turn_cbet", "fold_to_flop_cbet", "fold_to_turn_cbet", "wtsd", "w_sd", "wwsf", "afq",
	)
	all := setOf()
	for _, m := range metricRegistry {
		all[m.ID] = struct{}{}
	}
	return []MetricPreset{
		{Name: "Beginner", ButtonText: lang.X("preset.beginner", "Beginner"), MetricIDs: beginner},
		{Name: "Advanced", ButtonText: lang.X("preset.advanced", "Advanced"), MetricIDs: advanced},
		{Name: "Leak Focus", ButtonText: lang.X("preset.leak_focus", "Leak Focus"), MetricIDs: leak},
		{Name: "All", ButtonText: lang.X("preset.all", "All"), MetricIDs: all},
	}
}

func setOf(ids ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}

func colorForWinRate(rate float64) color.Color {
	if rate > 50 {
		return color.NRGBA{R: 0x4c, G: 0xaf, B: 0x50, A: 0xff}
	}
	if rate < 40 {
		return color.NRGBA{R: 0xf4, G: 0x43, B: 0x36, A: 0xff}
	}
	return theme.ForegroundColor()
}

func colorForVPIP(rate float64) color.Color {
	if rate > 35 {
		return color.NRGBA{R: 0xff, G: 0x98, B: 0x00, A: 0xff}
	}
	if rate >= 20 {
		return color.NRGBA{R: 0xff, G: 0xd6, B: 0x00, A: 0xff}
	}
	return theme.ForegroundColor()
}

func colorForProfit(profit int) color.Color {
	if profit > 0 {
		return color.NRGBA{R: 0x4c, G: 0xaf, B: 0x50, A: 0xff}
	}
	if profit < 0 {
		return color.NRGBA{R: 0xf4, G: 0x43, B: 0x36, A: 0xff}
	}
	return theme.ForegroundColor()
}

func statsMetricValue(s *stats.Stats, id stats.MetricID) MetricValue {
	if s == nil {
		return MetricValue{Display: "-", Color: theme.ForegroundColor(), Opportunities: 0}
	}
	m, ok := s.Metric(id)
	if !ok {
		return MetricValue{Display: "-", Color: theme.ForegroundColor(), Opportunities: 0}
	}
	switch m.Format {
	case stats.MetricFormatRatio:
		return MetricValue{Display: fmt.Sprintf("%.2f", m.Rate), Color: theme.ForegroundColor(), Opportunities: m.Opportunity}
	case stats.MetricFormatBBPer100:
		return MetricValue{Display: fmt.Sprintf("%.2f", m.Rate), Color: theme.ForegroundColor(), Opportunities: m.Opportunity}
	case stats.MetricFormatDiff, stats.MetricFormatPercent:
		return MetricValue{Display: fmt.Sprintf("%.1f%%", m.Rate), Color: theme.ForegroundColor(), Opportunities: m.Opportunity}
	default:
		return MetricValue{Display: fmt.Sprintf("%.1f%%", m.Rate), Color: theme.ForegroundColor(), Opportunities: m.Opportunity}
	}
}

type trendInsight struct {
	Level string
	Text  string
}

func buildTrendInsights(s *stats.Stats) []trendInsight {
	if s == nil || s.Metrics == nil {
		return nil
	}
	out := make([]trendInsight, 0, 6)
	vpip, okV := s.Metric(stats.MetricVPIP)
	pfr, okP := s.Metric(stats.MetricPFR)
	if okV && okP && vpip.Opportunity >= 200 {
		if vpip.Rate-pfr.Rate >= 12 {
			out = append(out, trendInsight{Level: "warn", Text: lang.X("insight.vpip_pfr_gap", "VPIP-PFR gap is large. You may be entering pots passively too often.")})
		}
	}
	foldSteal, okFS := s.Metric(stats.MetricFoldToSteal)
	if okFS && foldSteal.Opportunity >= 50 && foldSteal.Rate >= 70 {
		out = append(out, trendInsight{Level: "action", Text: lang.X("insight.fold_to_steal", "Fold to Steal is high. Review blind defense ranges and 3-bet/call mixes.")})
	}
	flopCbet, okFC := s.Metric(stats.MetricFlopCBet)
	turnCbet, okTC := s.Metric(stats.MetricTurnCBet)
	wwsf, okWW := s.Metric(stats.MetricWWSF)
	if okFC && okTC && okWW && flopCbet.Opportunity >= 50 && turnCbet.Opportunity >= 50 {
		if flopCbet.Rate >= 65 && turnCbet.Rate <= 35 && wwsf.Rate < 42 {
			out = append(out, trendInsight{Level: "warn", Text: lang.X("insight.over_cbet", "High flop c-bet but low turn follow-through. You may be over-cbetting flop then giving up.")})
		}
	}
	wtsd, okWT := s.Metric(stats.MetricWTSD)
	wsd, okWSD := s.Metric(stats.MetricWSD)
	if okWT && okWSD && wtsd.Opportunity >= 50 && wsd.Opportunity >= 50 {
		if wtsd.Rate >= 33 && wsd.Rate <= 45 {
			out = append(out, trendInsight{Level: "action", Text: lang.X("insight.thin_calls", "High WTSD with low W$SD suggests thin calls. Tighten showdown-bound bluff-catching.")})
		}
		if wtsd.Rate <= 20 && wwsf.Rate < 42 {
			out = append(out, trendInsight{Level: "info", Text: lang.X("insight.over_folding", "Low WTSD and low WWSF can indicate over-folding on later streets.")})
		}
	}
	return out
}

var metricRegistry = []MetricDefinition{
	{
		ID:             "hands",
		Label:          "Hands",
		Help:           lang.X("metric.hands.help", "Total complete hands included in current stats scope."),
		MinSamples:     200,
		ShowInOverview: true,
		ShowInPosition: true,
		OverviewValue: func(s *stats.Stats) MetricValue {
			if s == nil {
				return MetricValue{Display: "0", Color: theme.ForegroundColor(), Opportunities: 0}
			}
			return MetricValue{Display: fmt.Sprintf("%d", s.TotalHands), Color: theme.ForegroundColor(), Opportunities: s.TotalHands}
		},
		PositionValue: func(ps *stats.PositionStats) MetricValue {
			if ps == nil {
				return MetricValue{Display: "0", Color: theme.ForegroundColor(), Opportunities: 0}
			}
			return MetricValue{Display: fmt.Sprintf("%d", ps.Hands), Color: theme.ForegroundColor(), Opportunities: ps.Hands}
		},
	},
	{
		ID:             "winRate",
		Label:          "Win Rate",
		Help:           lang.X("metric.win_rate.help", "Won hands / total hands."),
		MinSamples:     200,
		ShowInOverview: true,
		ShowInPosition: true,
		OverviewValue: func(s *stats.Stats) MetricValue {
			if s == nil {
				return MetricValue{Display: "0.0%", Color: theme.ForegroundColor(), Opportunities: 0}
			}
			rate := s.WinRate()
			return MetricValue{Display: fmt.Sprintf("%.1f%%", rate), Color: colorForWinRate(rate), Opportunities: s.TotalHands}
		},
		PositionValue: func(ps *stats.PositionStats) MetricValue {
			if ps == nil {
				return MetricValue{Display: "0.0%", Color: theme.ForegroundColor(), Opportunities: 0}
			}
			rate := ps.WinRate()
			return MetricValue{Display: fmt.Sprintf("%.1f%%", rate), Color: colorForWinRate(rate), Opportunities: ps.Hands}
		},
	},
	{
		ID:             "profit",
		Label:          "Total Profit",
		Help:           lang.X("metric.profit.help", "Total chips won minus invested chips."),
		MinSamples:     200,
		ShowInOverview: true,
		ShowInPosition: true,
		OverviewValue: func(s *stats.Stats) MetricValue {
			if s == nil {
				return MetricValue{Display: "+0", Color: theme.ForegroundColor(), Opportunities: 0}
			}
			p := s.TotalPotWon - s.TotalInvested
			return MetricValue{Display: fmt.Sprintf("%+d", p), Color: colorForProfit(p), Opportunities: s.TotalHands}
		},
		PositionValue: func(ps *stats.PositionStats) MetricValue {
			if ps == nil {
				return MetricValue{Display: "+0", Color: theme.ForegroundColor(), Opportunities: 0}
			}
			p := ps.PotWon - ps.Invested
			return MetricValue{Display: fmt.Sprintf("%+d", p), Color: colorForProfit(p), Opportunities: ps.Hands}
		},
	},
}

func addStatsMetricDefinition(id stats.MetricID, label, help string, showPosition bool) {
	idCopy := id
	minSamples := 50
	if id == stats.MetricVPIP || id == stats.MetricPFR || id == stats.MetricGap || id == stats.MetricRFI || id == stats.MetricColdCall || id == stats.MetricWonWithoutSD || id == stats.MetricBBPer100 {
		minSamples = 200
	}
	metricRegistry = append(metricRegistry, MetricDefinition{
		ID:             string(id),
		Label:          label,
		Help:           help,
		MinSamples:     minSamples,
		ShowInOverview: true,
		ShowInPosition: showPosition,
		StatsMetricID:  &idCopy,
		OverviewValue: func(s *stats.Stats) MetricValue {
			v := statsMetricValue(s, id)
			if id == stats.MetricVPIP {
				v.Color = colorForVPIP(vRateOrZero(s, id))
			}
			return v
		},
		PositionValue: func(ps *stats.PositionStats) MetricValue {
			if ps == nil {
				return MetricValue{Display: "-", Color: theme.ForegroundColor(), Opportunities: 0}
			}
			switch id {
			case stats.MetricVPIP:
				rate := ps.VPIPRate()
				return MetricValue{Display: fmt.Sprintf("%.1f%%", rate), Color: colorForVPIP(rate), Opportunities: ps.Hands}
			case stats.MetricPFR:
				return MetricValue{Display: fmt.Sprintf("%.1f%%", ps.PFRRate()), Color: theme.ForegroundColor(), Opportunities: ps.Hands}
			case stats.MetricThreeBet:
				return MetricValue{Display: fmt.Sprintf("%.1f%%", ps.ThreeBetRate()), Color: theme.ForegroundColor(), Opportunities: ps.ThreeBetOpp}
			case stats.MetricFoldToThreeBet:
				return MetricValue{Display: fmt.Sprintf("%.1f%%", ps.FoldTo3BetRate()), Color: theme.ForegroundColor(), Opportunities: ps.FoldTo3BetOpp}
			case stats.MetricWSD:
				return MetricValue{Display: fmt.Sprintf("%.1f%%", ps.WSDRate()), Color: theme.ForegroundColor(), Opportunities: ps.Showdowns}
			default:
				return MetricValue{Display: "-", Color: theme.DisabledColor(), Opportunities: 0}
			}
		},
	})
}

func vRateOrZero(s *stats.Stats, id stats.MetricID) float64 {
	if s == nil {
		return 0
	}
	v, ok := s.Metric(id)
	if !ok {
		return 0
	}
	return v.Rate
}

func init() {
	addStatsMetricDefinition(stats.MetricVPIP, "VPIP", lang.X("metric.vpip.help", "Voluntarily Put Money In Pot. Preflop participation frequency."), true)
	addStatsMetricDefinition(stats.MetricPFR, "PFR", lang.X("metric.pfr.help", "Preflop raise frequency."), true)
	addStatsMetricDefinition(stats.MetricGap, "VPIP-PFR Gap", lang.X("metric.gap.help", "VPIP minus PFR. Larger gap implies more passive preflop entries."), false)
	addStatsMetricDefinition(stats.MetricRFI, "RFI", lang.X("metric.rfi.help", "Raise First In frequency."), false)
	addStatsMetricDefinition(stats.MetricColdCall, "Cold Call", lang.X("metric.cold_call.help", "Call preflop after someone opened while you were not yet in the pot."), false)
	addStatsMetricDefinition(stats.MetricThreeBet, "3Bet", lang.X("metric.three_bet.help", "3-bet frequency when a 3-bet opportunity is present."), true)
	addStatsMetricDefinition(stats.MetricFoldToThreeBet, "Fold to 3Bet", lang.X("metric.fold_to_three_bet.help", "Fold frequency when facing a 3-bet after opening."), true)
	addStatsMetricDefinition(stats.MetricSteal, "Steal Attempt", lang.X("metric.steal.help", "Open-raise attempt from steal positions when folded to you."), false)
	addStatsMetricDefinition(stats.MetricFoldToSteal, "Fold to Steal", lang.X("metric.fold_to_steal.help", "Fold frequency in blinds versus steal attempts."), false)
	addStatsMetricDefinition(stats.MetricFlopCBet, "Flop CBet", lang.X("metric.flop_cbet.help", "Continuation bet frequency on flop as preflop aggressor."), false)
	addStatsMetricDefinition(stats.MetricTurnCBet, "Turn CBet", lang.X("metric.turn_cbet.help", "Continuation bet frequency on turn."), false)
	addStatsMetricDefinition(stats.MetricRiverCBet, "River CBet", lang.X("metric.river_cbet.help", "Continuation bet frequency on river."), false)
	addStatsMetricDefinition(stats.MetricFoldToFlopCBet, "Fold to Flop CBet", lang.X("metric.fold_to_flop_cbet.help", "Fold frequency when facing flop c-bet."), false)
	addStatsMetricDefinition(stats.MetricFoldToTurnCBet, "Fold to Turn CBet", lang.X("metric.fold_to_turn_cbet.help", "Fold frequency when facing turn c-bet."), false)
	addStatsMetricDefinition(stats.MetricFoldToRiverCBet, "Fold to River CBet", lang.X("metric.fold_to_river_cbet.help", "Fold frequency when facing river c-bet."), false)
	addStatsMetricDefinition(stats.MetricWTSD, "WTSD", lang.X("metric.wtsd.help", "Went to showdown after seeing flop."), false)
	addStatsMetricDefinition(stats.MetricWSD, "W$SD", lang.X("metric.w_sd.help", "Won money at showdown."), true)
	addStatsMetricDefinition(stats.MetricWWSF, "WWSF", lang.X("metric.wwsf.help", "Won when saw flop."), false)
	addStatsMetricDefinition(stats.MetricAFq, "AFq", lang.X("metric.afq.help", "Aggression frequency: (bet+raise)/(actions postflop)."), false)
	addStatsMetricDefinition(stats.MetricAF, "AF", lang.X("metric.af.help", "Aggression factor: (bet+raise)/call."), false)
	addStatsMetricDefinition(stats.MetricCheckRaise, "Check-Raise", lang.X("metric.check_raise.help", "Check-raise frequency postflop."), false)
	addStatsMetricDefinition(stats.MetricDelayedCBet, "Delayed CBet", lang.X("metric.delayed_cbet.help", "Delayed continuation bet frequency (check flop, bet turn)."), false)
	addStatsMetricDefinition(stats.MetricWonWithoutSD, "Won without SD", lang.X("metric.won_without_sd.help", "Won hand without reaching showdown."), false)
	addStatsMetricDefinition(stats.MetricWonAtSD, "Won at SD", lang.X("metric.won_at_sd.help", "Won hand at showdown."), false)
	addStatsMetricDefinition(stats.MetricBBPer100, "bb/100", lang.X("metric.bb_per_100.help", "Net big blinds won per 100 hands."), false)
}
