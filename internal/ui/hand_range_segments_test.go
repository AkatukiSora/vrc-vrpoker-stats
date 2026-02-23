package ui

import (
	"math"
	"math/rand"
	"testing"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

func TestBuildRangeCellStopsZeroDealt(t *testing.T) {
	var counts [stats.RangeActionBucketCount]int
	counts[stats.RangeActionCheck] = 5

	stops, colors := buildRangeCellStops(counts, 0)
	if len(stops) != 0 {
		t.Fatalf("expected no stops when dealt=0, got %d", len(stops))
	}
	if len(colors) != 0 {
		t.Fatalf("expected no colors when dealt=0, got %d", len(colors))
	}
}

func TestBuildRangeCellStopsMonotonicAndPartialCoverage(t *testing.T) {
	var counts [stats.RangeActionBucketCount]int
	counts[stats.RangeActionCheck] = 8
	counts[stats.RangeActionCall] = 3
	counts[stats.RangeActionBetHalf] = 5
	counts[stats.RangeActionFold] = 2

	stops, colors := buildRangeCellStops(counts, 20)
	if len(stops) != 4 {
		t.Fatalf("expected 4 stops, got %d", len(stops))
	}
	if len(colors) != 4 {
		t.Fatalf("expected 4 colors, got %d", len(colors))
	}

	prev := float32(0)
	for i, stop := range stops {
		if stop <= prev {
			t.Fatalf("stops must be strictly increasing: stop[%d]=%f prev=%f", i, stop, prev)
		}
		if stop > 1 {
			t.Fatalf("stop[%d] must be <= 1, got %f", i, stop)
		}
		prev = stop
	}

	if math.Abs(float64(stops[len(stops)-1]-0.9)) > 1e-6 {
		t.Fatalf("expected last stop to be 0.9, got %f", stops[len(stops)-1])
	}
}

func TestBuildRangeCellStopsClampsAtOne(t *testing.T) {
	var counts [stats.RangeActionBucketCount]int
	counts[stats.RangeActionCheck] = 10
	counts[stats.RangeActionCall] = 10
	counts[stats.RangeActionFold] = 10

	stops, _ := buildRangeCellStops(counts, 12)
	if len(stops) == 0 {
		t.Fatal("expected at least one stop")
	}
	if stops[len(stops)-1] != 1 {
		t.Fatalf("expected last stop to clamp to 1, got %f", stops[len(stops)-1])
	}
}

func TestBuildRangeCellStopsRandomizedInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < 2000; i++ {
		dealt := rng.Intn(400) + 1
		var counts [stats.RangeActionBucketCount]int
		for b := 0; b < int(stats.RangeActionBucketCount); b++ {
			counts[b] = rng.Intn(400)
		}

		stops, colors := buildRangeCellStops(counts, dealt)
		if len(stops) != len(colors) {
			t.Fatalf("stops/colors length mismatch: %d vs %d", len(stops), len(colors))
		}
		if len(stops) > len(actionVisuals) {
			t.Fatalf("unexpected stop count: %d", len(stops))
		}

		prev := float32(0)
		for idx, stop := range stops {
			if math.IsNaN(float64(stop)) || math.IsInf(float64(stop), 0) {
				t.Fatalf("invalid stop value at %d: %f", idx, stop)
			}
			if stop <= prev {
				t.Fatalf("stops not strictly increasing at %d: %f <= %f", idx, stop, prev)
			}
			if stop < 0 || stop > 1 {
				t.Fatalf("stop out of range at %d: %f", idx, stop)
			}
			prev = stop
		}

		totalVisual := 0
		for _, av := range actionVisuals {
			totalVisual += actionCountForVisual(counts, av)
		}
		expectedLast := float32(totalVisual) / float32(dealt)
		if expectedLast > 1 {
			expectedLast = 1
		}

		if len(stops) == 0 {
			if expectedLast > 0 {
				t.Fatalf("expected at least one stop, got none (expectedLast=%f)", expectedLast)
			}
			continue
		}

		if math.Abs(float64(stops[len(stops)-1]-expectedLast)) > 1e-6 {
			t.Fatalf("last stop mismatch: got=%f expected=%f", stops[len(stops)-1], expectedLast)
		}
	}
}
