package discord

import (
	"testing"
	"time"
)

// bratislava is the zone the tests read naive times in. Chosen because it observes
// daylight saving, which is the case a fixed offset gets wrong.
func bratislava(t *testing.T) *time.Location {
	t.Helper()

	loc, err := time.LoadLocation("Europe/Bratislava")
	if err != nil {
		t.Skipf("no tzdata for Europe/Bratislava: %v", err)
	}
	return loc
}

func TestParseEventTimeReadsTheShorthands(t *testing.T) {
	loc := bratislava(t)
	// A Tuesday, 19:00 local.
	now := time.Date(2026, time.September, 1, 19, 0, 0, 0, loc)

	for _, tc := range []struct {
		name string
		in   string
		want time.Time
	}{
		{"tonight", "tonight 20:00", time.Date(2026, time.September, 1, 20, 0, 0, 0, loc)},
		{"today", "today 21:30", time.Date(2026, time.September, 1, 21, 30, 0, 0, loc)},
		{"tomorrow", "tomorrow 20:00", time.Date(2026, time.September, 2, 20, 0, 0, 0, loc)},
		{"weekday short", "sat 20:00", time.Date(2026, time.September, 5, 20, 0, 0, 0, loc)},
		{"weekday long", "saturday 20:00", time.Date(2026, time.September, 5, 20, 0, 0, 0, loc)},
		{"dotted clock", "tomorrow 20.30", time.Date(2026, time.September, 2, 20, 30, 0, 0, loc)},
		{"bare clock later today", "20:00", time.Date(2026, time.September, 1, 20, 0, 0, 0, loc)},
		{"iso", "2026-09-10 20:00", time.Date(2026, time.September, 10, 20, 0, 0, 0, loc)},
		{"dotted date", "10.09.2026 20:00", time.Date(2026, time.September, 10, 20, 0, 0, 0, loc)},
		{"case insensitive", "Tomorrow 20:00", time.Date(2026, time.September, 2, 20, 0, 0, 0, loc)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEventTime(tc.in, now, loc)
			if err != nil {
				t.Fatalf("parsing %q: %v", tc.in, err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("parsed %q as %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

// A bare clock time that has already passed means the next one, not one in the past.
// Typing "20:00" at nine in the evening is tomorrow's raid.
func TestABareClockTimeThatPassedMeansTomorrow(t *testing.T) {
	loc := bratislava(t)
	now := time.Date(2026, time.September, 1, 21, 0, 0, 0, loc)

	got, err := parseEventTime("20:00", now, loc)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	want := time.Date(2026, time.September, 2, 20, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("parsed as %s, want %s", got, want)
	}
}

// Naming today's weekday means next week. Nobody types "saturday" on Saturday
// afternoon to mean a raid that started in the morning.
func TestTodaysWeekdayMeansNextWeek(t *testing.T) {
	loc := bratislava(t)
	// A Saturday, 14:00.
	now := time.Date(2026, time.September, 5, 14, 0, 0, 0, loc)

	got, err := parseEventTime("saturday 10:00", now, loc)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	want := time.Date(2026, time.September, 12, 10, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("parsed as %s, want next Saturday %s", got, want)
	}
}

// A written offset is the raid lead being explicit, and beats the guild's zone.
func TestAnExplicitOffsetWinsOverTheGuildZone(t *testing.T) {
	loc := bratislava(t)
	now := time.Date(2026, time.September, 1, 19, 0, 0, 0, loc)

	got, err := parseEventTime("2026-09-10 20:00 +00:00", now, loc)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	want := time.Date(2026, time.September, 10, 20, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parsed as %s, want %s", got, want)
	}
}

// The zone is IANA rather than a fixed offset precisely so this works: a raid booked
// in October is one hour off if the offset was frozen in September.
func TestATimeAfterTheClocksChangeUsesTheRightOffset(t *testing.T) {
	loc := bratislava(t)
	now := time.Date(2026, time.September, 1, 19, 0, 0, 0, loc)

	summer, err := parseEventTime("2026-09-10 20:00", now, loc)
	if err != nil {
		t.Fatalf("parsing summer date: %v", err)
	}
	winter, err := parseEventTime("2026-11-10 20:00", now, loc)
	if err != nil {
		t.Fatalf("parsing winter date: %v", err)
	}

	_, summerOffset := summer.Zone()
	_, winterOffset := winter.Zone()
	if summerOffset == winterOffset {
		t.Errorf("both offsets are %d, want daylight saving to have moved one", summerOffset)
	}
}

func TestUnreadableTimesAreReported(t *testing.T) {
	loc := bratislava(t)
	now := time.Date(2026, time.September, 1, 19, 0, 0, 0, loc)

	for _, raw := range []string{"", "next tuesday-ish", "yesterday 20:00", "sat", "25:00"} {
		if _, err := parseEventTime(raw, now, loc); err == nil {
			t.Errorf("parsing %q succeeded, want an error the raid lead can act on", raw)
		}
	}
}

func TestTheDefaultDeadlineIsADayBeforeTheRaid(t *testing.T) {
	now := time.Date(2026, time.September, 1, 19, 0, 0, 0, time.UTC)
	startsAt := time.Date(2026, time.September, 10, 20, 0, 0, 0, time.UTC)

	got := defaultDeadline(startsAt, now)
	want := time.Date(2026, time.September, 9, 20, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("deadline = %s, want %s", got, want)
	}
}

// Closing signups a day before a raid that is three hours away would close them before
// they opened, so they run until the pull instead.
func TestASoonRaidKeepsSignupsOpenUntilItStarts(t *testing.T) {
	now := time.Date(2026, time.September, 1, 19, 0, 0, 0, time.UTC)
	startsAt := time.Date(2026, time.September, 1, 22, 0, 0, 0, time.UTC)

	if got := defaultDeadline(startsAt, now); !got.Equal(startsAt) {
		t.Errorf("deadline = %s, want the start time %s", got, startsAt)
	}
}

// The late modal anchors the parse on the raid's start, not on now. A raider answering
// at lunchtime who types "20:30" means half an hour into tonight's raid; anchored on
// now they would get half past eight and a late_until before the pull.
func TestAnArrivalTimeIsReadAgainstTheRaidStartNotTheClock(t *testing.T) {
	startsAt := time.Date(2026, time.September, 1, 20, 0, 0, 0, time.UTC)

	got, err := parseEventTime("20:30", startsAt, time.UTC)
	if err != nil {
		t.Fatalf("parseEventTime() error = %v", err)
	}
	want := time.Date(2026, time.September, 1, 20, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("arrival = %s, want %s", got, want)
	}
}

// A raid starting at 22:00 and a raider arriving at 01:00 is the next morning, not
// twenty-one hours before the pull.
func TestAnArrivalTimeAfterMidnightLandsOnTheFollowingDay(t *testing.T) {
	startsAt := time.Date(2026, time.September, 1, 22, 0, 0, 0, time.UTC)

	got, err := parseEventTime("01:00", startsAt, time.UTC)
	if err != nil {
		t.Fatalf("parseEventTime() error = %v", err)
	}
	want := time.Date(2026, time.September, 2, 1, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("arrival = %s, want %s", got, want)
	}
}
