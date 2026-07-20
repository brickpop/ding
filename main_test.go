package main

import (
	"testing"
	"time"
)

// ── parseWeekday ──────────────────────────────────────────────

func TestParseWeekday(t *testing.T) {
	cases := []struct {
		input string
		want  time.Weekday
	}{
		{"Monday", time.Monday},
		{"monday", time.Monday},
		{"MONDAY", time.Monday},
		{"  Tuesday  ", time.Tuesday},
		{"Wednesday", time.Wednesday},
		{"Thursday", time.Thursday},
		{"Friday", time.Friday},
		{"Saturday", time.Saturday},
		{"Sunday", time.Sunday},
	}
	for _, c := range cases {
		got, err := parseWeekday(c.input)
		if err != nil {
			t.Fatalf("parseWeekday(%q): unexpected error: %v", c.input, err)
		}
		if got != c.want {
			t.Errorf("parseWeekday(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestParseWeekdayBad(t *testing.T) {
	for _, input := range []string{"", "Mondai", "Fridy", "8"} {
		_, err := parseWeekday(input)
		if err == nil {
			t.Errorf("parseWeekday(%q): expected error, got nil", input)
		}
	}
}

// ── parseTime ─────────────────────────────────────────────────

func TestParseTime(t *testing.T) {
	cases := []struct {
		input string
		h, m  int
	}{
		{"09:00", 9, 0},
		{"21:30", 21, 30},
		{"00:00", 0, 0},
		{"23:59", 23, 59},
		{"8:5", 8, 5},
	}
	for _, c := range cases {
		h, m, err := parseTime(c.input)
		if err != nil {
			t.Fatalf("parseTime(%q): unexpected error: %v", c.input, err)
		}
		if h != c.h || m != c.m {
			t.Errorf("parseTime(%q) = %d:%d, want %d:%d", c.input, h, m, c.h, c.m)
		}
	}
}

func TestParseTimeBad(t *testing.T) {
	for _, input := range []string{"", "25:00", "12:60", "abc", "9:00:00", ":30"} {
		_, _, err := parseTime(input)
		if err == nil {
			t.Errorf("parseTime(%q): expected error, got nil", input)
		}
	}
}

// ── collectWeekdays ───────────────────────────────────────────

func TestCollectWeekdays(t *testing.T) {
	n := Notification{
		Weekday:  "Monday",
		Weekdays: []string{"Wednesday", "Friday"},
	}
	set := n.collectWeekdays()
	if len(set) != 3 {
		t.Fatalf("expected 3 weekdays, got %d", len(set))
	}
	if !set[time.Monday] || !set[time.Wednesday] || !set[time.Friday] {
		t.Errorf("unexpected set: %v", set)
	}
}

func TestCollectWeekdaysDedup(t *testing.T) {
	n := Notification{
		Weekday:  "Monday",
		Weekdays: []string{"Monday", "monday"},
	}
	set := n.collectWeekdays()
	if len(set) != 1 {
		t.Fatalf("expected 1 weekday after dedup, got %d", len(set))
	}
	if !set[time.Monday] {
		t.Error("expected Monday in set")
	}
}

func TestCollectWeekdaysEmpty(t *testing.T) {
	n := Notification{}
	set := n.collectWeekdays()
	if len(set) != 0 {
		t.Fatalf("expected empty set, got %d entries", len(set))
	}
}

func TestCollectWeekdaysPluralOnly(t *testing.T) {
	n := Notification{Weekdays: []string{"Saturday", "Sunday"}}
	set := n.collectWeekdays()
	if len(set) != 2 {
		t.Fatalf("expected 2 weekdays, got %d", len(set))
	}
}

// ── collectTimes ──────────────────────────────────────────────

func TestCollectTimes(t *testing.T) {
	n := Notification{Time: "08:00", Times: []string{"20:00"}}
	got := n.collectTimes()
	if len(got) != 2 || got[0] != "08:00" || got[1] != "20:00" {
		t.Errorf("expected [08:00 20:00], got %v", got)
	}
}

func TestCollectTimesDedup(t *testing.T) {
	n := Notification{Time: "09:00", Times: []string{"09:00", "21:00"}}
	got := n.collectTimes()
	if len(got) != 2 || got[0] != "09:00" || got[1] != "21:00" {
		t.Errorf("expected [09:00 21:00], got %v", got)
	}
}

func TestCollectTimesSingularOnly(t *testing.T) {
	n := Notification{Time: "12:00"}
	got := n.collectTimes()
	if len(got) != 1 || got[0] != "12:00" {
		t.Errorf("expected [12:00], got %v", got)
	}
}

func TestCollectTimesPluralOnly(t *testing.T) {
	n := Notification{Times: []string{"07:00", "19:00"}}
	got := n.collectTimes()
	if len(got) != 2 {
		t.Errorf("expected 2 times, got %v", got)
	}
}

func TestCollectTimesEmpty(t *testing.T) {
	n := Notification{}
	got := n.collectTimes()
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// ── resolvedNotif.isDue ───────────────────────────────────────

func testLoc(t *testing.T) *time.Location {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// makeTime builds a time in the given location.
func makeTime(t *testing.T, loc *time.Location, year int, month time.Month, day, hour, min int) time.Time {
	return time.Date(year, month, day, hour, min, 0, 0, loc)
}

func TestIsDueEveryDaySingleTime(t *testing.T) {
	loc := testLoc(t)
	r := resolvedNotif{
		title:    "test",
		weekdays: nil, // every day
		times:    []struct{ h, m int }{{9, 0}},
	}

	// 2025-07-14 is a Monday at 09:00 — should fire
	now := makeTime(t, loc, 2025, 7, 14, 9, 0)
	due, matched := r.isDue(now, loc)
	if !due {
		t.Error("expected due")
	}
	if matched != "09:00" {
		t.Errorf("matched=%q, want \"09:00\"", matched)
	}

	// Same day, wrong time — should not fire
	now = makeTime(t, loc, 2025, 7, 14, 10, 0)
	due, _ = r.isDue(now, loc)
	if due {
		t.Error("expected not due at 10:00")
	}

	// Different day, right time — should fire (every day)
	now = makeTime(t, loc, 2025, 7, 15, 9, 0) // Tuesday
	due, _ = r.isDue(now, loc)
	if !due {
		t.Error("expected due on Tuesday (every day)")
	}
}

func TestIsDueEveryDayMultipleTimes(t *testing.T) {
	loc := testLoc(t)
	r := resolvedNotif{
		title:    "meds",
		weekdays: nil,
		times: []struct{ h, m int }{
			{8, 0},
			{20, 0},
		},
	}

	// Should fire at both times
	for _, wantTime := range []int{8, 20} {
		now := makeTime(t, loc, 2025, 7, 14, wantTime, 0)
		due, _ := r.isDue(now, loc)
		if !due {
			t.Errorf("expected due at %d:00", wantTime)
		}
	}

	// Should not fire at other times
	now := makeTime(t, loc, 2025, 7, 14, 12, 0)
	due, _ := r.isDue(now, loc)
	if due {
		t.Error("expected not due at 12:00")
	}
}

func TestIsDueSpecificWeekday(t *testing.T) {
	loc := testLoc(t)
	r := resolvedNotif{
		title:    "plants",
		weekdays: map[time.Weekday]bool{time.Friday: true},
		times:    []struct{ h, m int }{{21, 0}},
	}

	// 2025-07-18 is a Friday at 21:00 — should fire
	now := makeTime(t, loc, 2025, 7, 18, 21, 0)
	due, _ := r.isDue(now, loc)
	if !due {
		t.Error("expected due on Friday 21:00")
	}

	// 2025-07-18 is a Friday at 09:00 — wrong time
	now = makeTime(t, loc, 2025, 7, 18, 9, 0)
	due, _ = r.isDue(now, loc)
	if due {
		t.Error("expected not due at wrong time on Friday")
	}

	// 2025-07-14 is a Monday at 21:00 — wrong day
	now = makeTime(t, loc, 2025, 7, 14, 21, 0)
	due, _ = r.isDue(now, loc)
	if due {
		t.Error("expected not due on Monday")
	}
}

func TestIsDueMultipleWeekdays(t *testing.T) {
	loc := testLoc(t)
	r := resolvedNotif{
		title: "check-in",
		weekdays: map[time.Weekday]bool{
			time.Monday:    true,
			time.Wednesday: true,
			time.Friday:    true,
		},
		times: []struct{ h, m int }{{12, 0}},
	}

	// Monday — should fire
	now := makeTime(t, loc, 2025, 7, 14, 12, 0)
	due, _ := r.isDue(now, loc)
	if !due {
		t.Error("expected due on Monday")
	}

	// Wednesday — should fire
	now = makeTime(t, loc, 2025, 7, 16, 12, 0)
	due, _ = r.isDue(now, loc)
	if !due {
		t.Error("expected due on Wednesday")
	}

	// Tuesday — should NOT fire
	now = makeTime(t, loc, 2025, 7, 15, 12, 0)
	due, _ = r.isDue(now, loc)
	if due {
		t.Error("expected not due on Tuesday")
	}

	// Friday — should fire
	now = makeTime(t, loc, 2025, 7, 18, 12, 0)
	due, _ = r.isDue(now, loc)
	if !due {
		t.Error("expected due on Friday")
	}
}

func TestIsDueWeekdayAndMultipleTimes(t *testing.T) {
	loc := testLoc(t)
	r := resolvedNotif{
		title: "trash",
		weekdays: map[time.Weekday]bool{
			time.Wednesday: true,
			time.Saturday:  true,
		},
		times: []struct{ h, m int }{{6, 30}, {18, 30}},
	}

	// Wednesday at 06:30 — should fire
	now := makeTime(t, loc, 2025, 7, 16, 6, 30)
	due, _ := r.isDue(now, loc)
	if !due {
		t.Error("expected due Wed 06:30")
	}

	// Wednesday at 18:30 — should fire
	now = makeTime(t, loc, 2025, 7, 16, 18, 30)
	due, _ = r.isDue(now, loc)
	if !due {
		t.Error("expected due Wed 18:30")
	}

	// Saturday at 06:30 — should fire
	now = makeTime(t, loc, 2025, 7, 19, 6, 30)
	due, _ = r.isDue(now, loc)
	if !due {
		t.Error("expected due Sat 06:30")
	}

	// Saturday at 12:00 — wrong time
	now = makeTime(t, loc, 2025, 7, 19, 12, 0)
	due, _ = r.isDue(now, loc)
	if due {
		t.Error("expected not due Sat 12:00")
	}

	// Thursday at 06:30 — wrong day
	now = makeTime(t, loc, 2025, 7, 17, 6, 30)
	due, _ = r.isDue(now, loc)
	if due {
		t.Error("expected not due on Thursday")
	}
}

func TestIsDueWrongMinute(t *testing.T) {
	loc := testLoc(t)
	r := resolvedNotif{
		title:    "test",
		weekdays: nil,
		times:    []struct{ h, m int }{{9, 15}},
	}

	// 09:14 — one minute early
	now := makeTime(t, loc, 2025, 7, 14, 9, 14)
	due, _ := r.isDue(now, loc)
	if due {
		t.Error("expected not due at 09:14")
	}

	// 09:15 — exact match
	now = makeTime(t, loc, 2025, 7, 14, 9, 15)
	due, _ = r.isDue(now, loc)
	if !due {
		t.Error("expected due at 09:15")
	}

	// 09:16 — one minute late
	now = makeTime(t, loc, 2025, 7, 14, 9, 16)
	due, _ = r.isDue(now, loc)
	if due {
		t.Error("expected not due at 09:16")
	}
}

// ── Config validation edge cases (via toml.DecodeFile + resolve) ──

func TestConfigNoTime(t *testing.T) {
	// This is tested by the binary's fatal exit; we verify collectTimes returns empty.
	n := Notification{Title: "bad"}
	if len(n.collectTimes()) != 0 {
		t.Error("expected empty times")
	}
}

func TestConfigNoWeekday(t *testing.T) {
	n := Notification{Title: "daily"}
	set := n.collectWeekdays()
	if len(set) != 0 {
		t.Error("expected empty weekdays (every day)")
	}
}
