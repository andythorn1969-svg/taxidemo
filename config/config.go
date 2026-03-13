// ============================================================
// Taxidemo - Southend Taxi Cooperative Dispatch Demo
// Package config: Cooperative business policy configuration
//
// This file contains values that reflect cooperative policy decisions.
// They can be adjusted by the cooperative membership (or their nominated
// technical representative) without needing to understand the rest of
// the codebase. Each constant is documented with who would change it
// and under what circumstances.
// ============================================================

package config

import "time"

// Config holds all tunable policy values for the cooperative.
type Config struct {

	// ── Customer flagging policy ─────────────────────────────────────────────
	// Changed by: cooperative board / membership vote.
	// These thresholds control when a customer is automatically flagged for
	// excessive no-shows. Both conditions must be true simultaneously.

	// NoShowMinCount is the minimum number of absolute no-shows a customer must
	// have before the flagging system will consider them at all. This prevents
	// penalising a customer who no-showed once from a total of two bookings.
	// Recommended range: 2–5.
	NoShowMinCount int

	// NoShowMinRate is the minimum no-show rate (as a fraction) required to
	// trigger a flag. 0.15 means 15% of bookings must have been no-shows.
	// Recommended range: 0.10–0.25.
	NoShowMinRate float64

	// NoShowWeight is the score contribution of each confirmed no-show (driver
	// arrived at pickup, nobody present). Default 1.0 — counts in full.
	NoShowWeight float64

	// CancellationWeight is the score contribution of each pre-arrival
	// cancellation (passenger cancelled before driver reached pickup).
	// Default 0.3 — counts at reduced weight relative to a no-show.
	CancellationWeight float64

	// ── Late-night / weekend exclusion ──────────────────────────────────────
	// Changed by: cooperative board.
	// Bookings placed during these windows are excluded from the no-show count
	// (they still contribute to the total booking count). This reflects the
	// cooperative's policy that late-night no-shows on weekends are often
	// caused by circumstances beyond the passenger's control and should not
	// count against them.

	// ExcludeLateWeekend enables or disables the late-night exclusion window
	// entirely. Set to false to count all bookings equally.
	ExcludeLateWeekend bool

	// LateNightStartHour is the hour (24-hour clock, inclusive) at which the
	// late-night exclusion window begins. Default 22 = 10pm.
	LateNightStartHour int

	// LateNightEndHour is the hour (24-hour clock, inclusive) at which the
	// late-night exclusion window ends the following morning. Default 4 = 4am.
	LateNightEndHour int

	// LateNightDays lists the days of the week on which the late-night window
	// applies. Default: Friday and Saturday nights (into Saturday / Sunday
	// early morning).
	LateNightDays []time.Weekday

	// ── Prebook scheduler ───────────────────────────────────────────────────
	// Changed by: operations manager.
	// Controls how far in advance of the requested pickup time the scheduler
	// dispatches a prebook to a driver.

	// DefaultApproachMinutes is the fallback approach time (in minutes) used
	// when a pickup zone has no AverageApproachMinutes value set. The scheduler
	// dispatches a prebook this many minutes before the requested pickup time.
	// Increase if drivers are regularly arriving late to prebooks.
	DefaultApproachMinutes int

	// ── Driver movement simulation ──────────────────────────────────────────
	// Changed by: developer / demo operator.
	// These values control the demo simulation only and have no effect on a
	// real dispatch system.

	// SimTickSeconds is how often (in seconds) the simulation advances driver
	// positions. Lower values give smoother movement but higher CPU use.
	SimTickSeconds int

	// SimJourneyMinutes is how long (in minutes) a simulated journey takes from
	// dispatch to destination, regardless of actual geographic distance.
	// Increase to slow the demo down; decrease to speed it up.
	SimJourneyMinutes int
}

// Policy is the single shared instance of cooperative configuration.
// Import this package and read values as config.Policy.FieldName.
var Policy = Config{
	// Customer flagging
	NoShowMinCount:     3,
	NoShowMinRate:      0.15,
	NoShowWeight:       1.0,
	CancellationWeight: 0.3,

	// Late-night weekend exclusion
	ExcludeLateWeekend: true,
	LateNightStartHour: 22,
	LateNightEndHour:   4,
	LateNightDays:      []time.Weekday{time.Friday, time.Saturday},

	// Prebook scheduler
	DefaultApproachMinutes: 10,

	// Driver simulation
	SimTickSeconds:    5,
	SimJourneyMinutes: 2,
}
