package ui

import (
	"log/slog"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

type metricCategoryID string

const (
	metricCategoryResult   metricCategoryID = "result"
	metricCategoryPreflop  metricCategoryID = "preflop"
	metricCategoryPostflop metricCategoryID = "postflop"
	metricCategoryShowdown metricCategoryID = "showdown"
)

type metricThreshold struct {
	Min  int
	Good int
}

type metricCatalogEntry struct {
	Category  metricCategoryID
	Threshold metricThreshold
}

const (
	defaultMetricMinSamples  = 50
	defaultMetricGoodSamples = 200
)

var metricCatalog = map[string]metricCatalogEntry{
	"hands":                      {Category: metricCategoryResult, Threshold: metricThreshold{Min: 300, Good: 5000}},
	"profit":                     {Category: metricCategoryResult, Threshold: metricThreshold{Min: 300, Good: 5000}},
	string(stats.MetricVPIP):     {Category: metricCategoryPreflop, Threshold: metricThreshold{Min: 30, Good: 200}},
	string(stats.MetricPFR):      {Category: metricCategoryPreflop, Threshold: metricThreshold{Min: 30, Good: 200}},
	string(stats.MetricGap):      {Category: metricCategoryPreflop, Threshold: metricThreshold{Min: 30, Good: 200}},
	string(stats.MetricRFI):      {Category: metricCategoryPreflop, Threshold: metricThreshold{Min: 50, Good: 300}},
	string(stats.MetricSteal):    {Category: metricCategoryPreflop, Threshold: metricThreshold{Min: 50, Good: 300}},
	string(stats.MetricThreeBet): {Category: metricCategoryPreflop, Threshold: metricThreshold{Min: 50, Good: 300}},
	string(stats.MetricThreeBetVsSteal): {
		Category:  metricCategoryPreflop,
		Threshold: metricThreshold{Min: 40, Good: 200},
	},
	string(stats.MetricFoldToThreeBet): {Category: metricCategoryPreflop, Threshold: metricThreshold{Min: 30, Good: 200}},
	string(stats.MetricFourBet):        {Category: metricCategoryPreflop, Threshold: metricThreshold{Min: 20, Good: 120}},
	string(stats.MetricSqueeze):        {Category: metricCategoryPreflop, Threshold: metricThreshold{Min: 20, Good: 120}},
	string(stats.MetricFoldToSteal):    {Category: metricCategoryPreflop, Threshold: metricThreshold{Min: 50, Good: 300}},
	string(stats.MetricFoldBBToSteal):  {Category: metricCategoryPreflop, Threshold: metricThreshold{Min: 50, Good: 300}},
	string(stats.MetricFoldSBToSteal):  {Category: metricCategoryPreflop, Threshold: metricThreshold{Min: 50, Good: 300}},
	string(stats.MetricFlopCBet):       {Category: metricCategoryPostflop, Threshold: metricThreshold{Min: 40, Good: 200}},
	string(stats.MetricTurnCBet):       {Category: metricCategoryPostflop, Threshold: metricThreshold{Min: 30, Good: 150}},
	string(stats.MetricDelayedCBet):    {Category: metricCategoryPostflop, Threshold: metricThreshold{Min: 30, Good: 150}},
	string(stats.MetricFoldToFlopCBet): {Category: metricCategoryPostflop, Threshold: metricThreshold{Min: 40, Good: 200}},
	string(stats.MetricFoldToTurnCBet): {Category: metricCategoryPostflop, Threshold: metricThreshold{Min: 30, Good: 150}},
	string(stats.MetricWTSD):           {Category: metricCategoryShowdown, Threshold: metricThreshold{Min: 200, Good: 1000}},
	string(stats.MetricWWSF):           {Category: metricCategoryShowdown, Threshold: metricThreshold{Min: 200, Good: 1000}},
	string(stats.MetricWSD):            {Category: metricCategoryShowdown, Threshold: metricThreshold{Min: 50, Good: 300}},
	string(stats.MetricAFq):            {Category: metricCategoryPostflop, Threshold: metricThreshold{Min: 80, Good: 400}},
	string(stats.MetricAF):             {Category: metricCategoryPostflop, Threshold: metricThreshold{Min: 100, Good: 500}},
	string(stats.MetricWonWithoutSD):   {Category: metricCategoryShowdown, Threshold: metricThreshold{Min: 10000, Good: 50000}},
	string(stats.MetricBBPer100):       {Category: metricCategoryResult, Threshold: metricThreshold{Min: 10000, Good: 50000}},
}

func init() {
	missing := make([]string, 0)
	for _, def := range metricRegistry {
		if def.ID == "" {
			continue
		}
		if _, ok := metricCatalog[def.ID]; !ok {
			missing = append(missing, def.ID)
		}
		if def.StatsMetricID != nil {
			statsID := string(*def.StatsMetricID)
			if _, ok := metricCatalog[statsID]; !ok {
				missing = append(missing, statsID)
			}
		}
	}
	if len(missing) > 0 {
		slog.Warn("metric catalog missing entries", "metrics", missing)
	}
}

func metricCatalogEntryForID(metricID string) metricCatalogEntry {
	if entry, ok := metricCatalog[metricID]; ok {
		return entry
	}
	return metricCatalogEntry{
		Category:  metricCategoryShowdown,
		Threshold: metricThreshold{Min: defaultMetricMinSamples, Good: defaultMetricGoodSamples},
	}
}

func metricCategoryForMetricID(metricID string) metricCategoryID {
	return metricCatalogEntryForID(metricID).Category
}

func metricThresholdForMetricID(metricID string) metricThreshold {
	return metricCatalogEntryForID(metricID).Threshold
}
