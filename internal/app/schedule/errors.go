package schedule

import "errors"

var (
	// ErrBadCron means the expression will not parse.
	ErrBadCron = errors.New("schedule: cannot parse cron")
	// ErrBadTimezone means the location is not one the host knows.
	ErrBadTimezone = errors.New("schedule: unknown timezone")
)
