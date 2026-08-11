package schedule

import "errors"

var (
	// ErrBadCron means the expression will not parse.
	ErrBadCron = errors.New("schedule: cannot parse cron")
	// ErrBadTimezone means the location is not one the host knows.
	ErrBadTimezone = errors.New("schedule: unknown timezone")
	// ErrNoSchedule means nothing is published under that name.
	ErrNoSchedule = errors.New("schedule: no such schedule")
	// ErrRunning means the previous run of it has not finished.
	ErrRunning = errors.New("schedule: already running")
)
