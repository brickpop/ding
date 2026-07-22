package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// ── Config types ──────────────────────────────────────────────

type Config struct {
	NtfyURL       string         `toml:"ntfy_url"`
	Topic         string         `toml:"topic"`
	Timezone      string         `toml:"timezone"`
	AuthToken     string         `toml:"auth_token"`
	Notifications []Notification `toml:"notifications"`
}

type Notification struct {
	Title    string   `toml:"title"`
	Message  string   `toml:"message"`
	Topic    string   `toml:"topic"`     // optional — overrides global topic
	Weekday  string   `toml:"weekday"`   // optional, singular
	Weekdays []string `toml:"weekdays"`  // optional, plural — merged with weekday
	Time     string   `toml:"time"`      // optional, singular
	Times    []string `toml:"times"`     // optional, plural — merged with time
}

// ── NTFY sender (no JSON — ntfy uses headers + plain body) ──

// ── Helpers ───────────────────────────────────────────────────

var weekdayNames = map[string]time.Weekday{
	"monday":    time.Monday,
	"tuesday":   time.Tuesday,
	"wednesday": time.Wednesday,
	"thursday":  time.Thursday,
	"friday":    time.Friday,
	"saturday":  time.Saturday,
	"sunday":    time.Sunday,
}

func parseWeekday(name string) (time.Weekday, error) {
	if wd, ok := weekdayNames[strings.ToLower(strings.TrimSpace(name))]; ok {
		return wd, nil
	}
	return -1, fmt.Errorf("unknown weekday %q (want Monday..Sunday)", name)
}

// collectWeekdays merges weekday + weekdays[] into a deduplicated set.
func (n *Notification) collectWeekdays() map[time.Weekday]bool {
	set := make(map[time.Weekday]bool)
	if n.Weekday != "" {
		if wd, err := parseWeekday(n.Weekday); err == nil {
			set[wd] = true
		}
	}
	for _, w := range n.Weekdays {
		if wd, err := parseWeekday(w); err == nil {
			set[wd] = true
		}
	}
	return set
}

// collectTimes merges time + times[] into a deduplicated slice.
func (n *Notification) collectTimes() []string {
	seen := make(map[string]bool)
	var out []string
	if n.Time != "" {
		if !seen[n.Time] {
			seen[n.Time] = true
			out = append(out, n.Time)
		}
	}
	for _, t := range n.Times {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// parseTime parses a single "HH:MM" string.
func parseTime(raw string) (h, m int, err error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("bad format %q (want HH:MM)", raw)
	}
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
		return 0, 0, fmt.Errorf("bad hour in %q", raw)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
		return 0, 0, fmt.Errorf("bad minute in %q", raw)
	}
	if h < 0 || h > 23 {
		return 0, 0, fmt.Errorf("hour must be 0-23, got %d", h)
	}
	if m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("minute must be 0-59, got %d", m)
	}
	return h, m, nil
}

// ── Resolved notification (after merging singular/plural fields) ──

type resolvedNotif struct {
	title     string
	message   string
	topic     string                // empty = use global topic
	weekdays  map[time.Weekday]bool // empty = every day
	times     []struct{ h, m int }
}

func (r *resolvedNotif) isDue(now time.Time, loc *time.Location) (bool, string) {
	target := now.In(loc)

	// Check weekday(s) — if none set, matches every day
	if len(r.weekdays) > 0 && !r.weekdays[target.Weekday()] {
		return false, ""
	}

	// Check each configured time
	for _, t := range r.times {
		if target.Hour() == t.h && target.Minute() == t.m {
			return true, fmt.Sprintf("%02d:%02d", t.h, t.m)
		}
	}

	return false, ""
}

// ── NTFY sender ───────────────────────────────────────────────

const (
	maxRetries    = 10
	retryDelay    = 20 * time.Second
)

// sendNtfyWithRetry retries sending an ntfy notification up to maxRetries times
// with a retryDelay between each attempt.
func sendNtfyWithRetry(cfg *Config, topic, title, message string, dryRun bool) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := sendNtfy(cfg, topic, title, message, dryRun)
		if err == nil {
			if attempt > 1 {
				log.Printf("  retry succeeded on attempt %d/%d", attempt, maxRetries)
			}
			return nil
		}

		lastErr = err
		log.Printf("  attempt %d/%d failed: %v", attempt, maxRetries, err)

		if attempt < maxRetries {
			log.Printf("  retrying in %v...", retryDelay)
			time.Sleep(retryDelay)
		}
	}

	return fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

func sendNtfy(cfg *Config, topic, title, message string, dryRun bool) error {
	// Use per-notification topic if set, otherwise fall back to global
	if topic == "" {
		topic = cfg.Topic
	}

	url := fmt.Sprintf("%s/%s", strings.TrimRight(cfg.NtfyURL, "/"), topic)

	if dryRun {
		log.Printf("[DRY-RUN] POST %s  title=%q message=%q", url, title, message)
		return nil
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		url,
		strings.NewReader(message),
	)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Title", title)
	req.Header.Set("Priority", "3")
	if cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned %d", resp.StatusCode)
	}

	log.Printf("  sent (HTTP %d): %s", resp.StatusCode, title)
	return nil
}

// ── Main loop ─────────────────────────────────────────────────

func run(cfg *Config, resolved []resolvedNotif, dryRun bool) error {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return fmt.Errorf("load timezone %q: %w", cfg.Timezone, err)
	}

	log.Printf("started — %d notification(s), tz=%s, dry_run=%v",
		len(resolved), cfg.Timezone, dryRun)

	if len(resolved) == 0 {
		log.Println("no notifications defined")
		return nil
	}

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	check := func() {
		now := time.Now()
		target := now.In(loc)
		log.Printf("[%s local] checking...", target.Format(time.RFC3339))

		for i := range resolved {
			r := &resolved[i]
			due, matchedTime := r.isDue(now, loc)
			if due {
				log.Printf("  %s: DUE (at %s)", r.title, matchedTime)
				if err := sendNtfyWithRetry(cfg, r.topic, r.title, r.message, dryRun); err != nil {
					log.Printf("  ERROR (all retries exhausted): %v", err)
				}
			}
		}
	}

	check() // immediate check on start

	for range ticker.C {
		check()
	}

	return nil
}

// ── Entry point ───────────────────────────────────────────────

func main() {
	configPath := flag.String("config", "", "path to config file")
	dryRun := flag.Bool("dry-run", false, "print what would fire without sending")
	flag.Parse()

	if *configPath == "" {
		if _, err := os.Stat("./config.toml"); err == nil {
			*configPath = "./config.toml"
		} else if _, err := os.Stat("/etc/ding/config.toml"); err == nil {
			*configPath = "/etc/ding/config.toml"
		} else {
			log.Fatal("no config file found. Provide -config=path or place config.toml in the current directory")
		}
	}

	log.Printf("loading %s", *configPath)

	var cfg Config
	if _, err := toml.DecodeFile(*configPath, &cfg); err != nil {
		log.Fatalf("parse config: %v", err)
	}

	if cfg.NtfyURL == "" {
		log.Fatal("ntfy_url is required")
	}
	if cfg.Topic == "" {
		log.Fatal("topic is required")
	}
	if cfg.Timezone == "" {
		log.Fatal("timezone is required")
	}

	// Resolve notifications (merge singular/plural fields)
	resolved := make([]resolvedNotif, 0, len(cfg.Notifications))
	for i, n := range cfg.Notifications {
		if n.Title == "" {
			log.Fatalf("notifications[%d]: title is required", i)
		}

		times := n.collectTimes()
		if len(times) == 0 {
			log.Fatalf("notifications[%d]: time or times is required (e.g. time = \"09:00\")", i)
		}

		parsedTimes := make([]struct{ h, m int }, 0, len(times))
		for j, t := range times {
			h, m, err := parseTime(t)
			if err != nil {
				log.Fatalf("notifications[%d].times[%d]: %v", i, j, err)
			}
			parsedTimes = append(parsedTimes, struct{ h, m int }{h, m})
		}

		resolved = append(resolved, resolvedNotif{
			title:    n.Title,
			message:  n.Message,
			topic:    n.Topic,
			weekdays: n.collectWeekdays(),
			times:    parsedTimes,
		})
	}

	log.Printf("config OK: %s/%s, tz=%s, %d notification(s)",
		cfg.NtfyURL, cfg.Topic, cfg.Timezone, len(resolved))

	absPath, _ := filepath.Abs(*configPath)
	log.Printf("config: %s", absPath)

	if err := run(&cfg, resolved, *dryRun); err != nil {
		log.Fatalf("run: %v", err)
	}
}
