package discord

import (
	"fmt"
	"strings"
	"time"
)

// timeExample is what a parse failure shows back. Deliberately the shortest form that
// works, not the most explicit one: a raid lead typing a start time wants to type as
// little as possible.
const timeExample = "tomorrow 20:00"

// absoluteLayouts are tried in order. The first two carry their own offset and are
// unambiguous anywhere; the rest are read in the guild's zone.
var absoluteLayouts = []struct {
	layout string
	zoned  bool
}{
	{time.RFC3339, true},
	{"2006-01-02 15:04 -07:00", true},
	{"2006-01-02 15:04 -0700", true},
	{"2006-01-02 15:04", false},
	{"2006-01-02 1504", false},
	{"02.01.2006 15:04", false},
	{"02/01/2006 15:04", false},
}

var weekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "sun": time.Sunday,
	"monday": time.Monday, "mon": time.Monday,
	"tuesday": time.Tuesday, "tue": time.Tuesday, "tues": time.Tuesday,
	"wednesday": time.Wednesday, "wed": time.Wednesday,
	"thursday": time.Thursday, "thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday,
	"friday": time.Friday, "fri": time.Friday,
	"saturday": time.Saturday, "sat": time.Saturday,
}

// parseEventTime reads a raid time the way a raid lead would type one.
//
// now and loc are arguments rather than time.Now() and a package variable so that every
// case below is testable without waiting for Tuesday. loc is the guild's configured
// zone; a form that carries its own offset ignores it.
func parseEventTime(raw string, now time.Time, loc *time.Location) (time.Time, error) {
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" {
		return time.Time{}, fmt.Errorf("no time given")
	}

	for _, candidate := range absoluteLayouts {
		if candidate.zoned {
			if parsed, err := time.Parse(candidate.layout, raw); err == nil {
				return parsed, nil
			}
			continue
		}
		if parsed, err := time.ParseInLocation(candidate.layout, raw, loc); err == nil {
			return parsed, nil
		}
	}

	return parseRelativeTime(text, now.In(loc), loc)
}

// parseRelativeTime handles the shorthands: "tomorrow 20:00", "sat 20:00", and a bare
// "20:00" meaning the next time it is 20:00.
func parseRelativeTime(text string, now time.Time, loc *time.Location) (time.Time, error) {
	fields := strings.Fields(text)

	// A bare clock time means the next one to come round, so "20:00" typed at nine in
	// the evening is tomorrow rather than an hour ago.
	if len(fields) == 1 {
		clock, err := parseClock(fields[0])
		if err != nil {
			return time.Time{}, err
		}
		at := onDay(now, clock, loc)
		if !at.After(now) {
			at = at.AddDate(0, 0, 1)
		}
		return at, nil
	}

	if len(fields) != 2 {
		return time.Time{}, fmt.Errorf("cannot read %q as a time", text)
	}

	clock, err := parseClock(fields[1])
	if err != nil {
		return time.Time{}, err
	}

	switch day := fields[0]; day {
	case "today", "tonight":
		return onDay(now, clock, loc), nil
	case "tomorrow", "tmr":
		return onDay(now.AddDate(0, 0, 1), clock, loc), nil
	default:
		weekday, ok := weekdays[day]
		if !ok {
			return time.Time{}, fmt.Errorf("cannot read %q as a day", day)
		}
		return nextWeekday(now, weekday, clock, loc), nil
	}
}

// nextWeekday finds the coming occurrence of a weekday. Naming today's weekday means
// next week, not this morning: nobody types "saturday" on Saturday afternoon to mean a
// raid that already started.
func nextWeekday(now time.Time, weekday time.Weekday, clock clockTime, loc *time.Location) time.Time {
	ahead := (int(weekday) - int(now.Weekday()) + 7) % 7
	at := onDay(now.AddDate(0, 0, ahead), clock, loc)
	if !at.After(now) {
		at = at.AddDate(0, 0, 7)
	}
	return at
}

type clockTime struct {
	hour, minute int
}

func onDay(day time.Time, clock clockTime, loc *time.Location) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), clock.hour, clock.minute, 0, 0, loc)
}

// parseClock reads "20:00", "20.00" or "2000".
func parseClock(text string) (clockTime, error) {
	normalised := strings.ReplaceAll(text, ".", ":")
	for _, layout := range []string{"15:04", "1504"} {
		if parsed, err := time.Parse(layout, normalised); err == nil {
			return clockTime{hour: parsed.Hour(), minute: parsed.Minute()}, nil
		}
	}
	return clockTime{}, fmt.Errorf("cannot read %q as a time of day", text)
}

// defaultDeadline is when signups close if a raid lead did not say.
//
// An hour before the pull is the useful answer: people decide late, and a sheet that
// closes the evening before turns every ordinary change of plan into a late request a
// raid lead has to sign off. When the raid is sooner than that, closing signups an hour
// early would mean closing them before they opened, so they run until the pull instead.
func defaultDeadline(startsAt, now time.Time) time.Time {
	deadline := startsAt.Add(-time.Hour)
	if deadline.Before(now) {
		return startsAt
	}
	return deadline
}
