package cli

import (
	"testing"
	"time"
)

func TestParseScheduleSpecSupportsMultipleDailyTimes(t *testing.T) {
	spec, err := parseScheduleSpec(0, []string{"13:00, 05:00", "21:30:15"})
	if err != nil {
		t.Fatalf("parseScheduleSpec: %v", err)
	}
	if spec.Every != 0 {
		t.Fatalf("every = %s, want 0", spec.Every)
	}
	got := []string{spec.At[0].String(), spec.At[1].String(), spec.At[2].String()}
	want := []string{"05:00", "13:00", "21:30:15"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("at[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestScheduleSpecNextRunChoosesEarliestTime(t *testing.T) {
	loc := time.FixedZone("test", 8*3600)
	now := time.Date(2026, 7, 5, 4, 30, 0, 0, loc)
	spec := scheduleSpec{
		Every: 2 * time.Hour,
		At: []dailyTime{
			{Hour: 5},
			{Hour: 21},
		},
	}

	got := spec.nextRun(now)
	want := time.Date(2026, 7, 5, 5, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("nextRun = %s, want %s", got, want)
	}
}

func TestScheduleSpecNextRunRollsDailyTimeToTomorrow(t *testing.T) {
	loc := time.FixedZone("test", 8*3600)
	now := time.Date(2026, 7, 5, 22, 0, 0, 0, loc)
	spec := scheduleSpec{
		At: []dailyTime{
			{Hour: 5},
			{Hour: 21},
		},
	}

	got := spec.nextRun(now)
	want := time.Date(2026, 7, 6, 5, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("nextRun = %s, want %s", got, want)
	}
}
