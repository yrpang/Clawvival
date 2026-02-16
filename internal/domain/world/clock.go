package world

import "time"

type Phase string

const (
	PhaseDay   Phase = "day"
	PhaseNight Phase = "night"
)

type ClockConfig struct {
	StartAt       time.Time
	DayDuration   time.Duration
	NightDuration time.Duration
}

type Clock struct {
	cfg ClockConfig
}

func NewClock(cfg ClockConfig) Clock {
	if cfg.DayDuration <= 0 {
		cfg.DayDuration = 10 * time.Minute
	}
	if cfg.NightDuration <= 0 {
		cfg.NightDuration = 5 * time.Minute
	}
	if cfg.StartAt.IsZero() {
		cfg.StartAt = time.Unix(0, 0)
	}
	return Clock{cfg: cfg}
}

func DefaultClock() Clock {
	return NewClock(ClockConfig{})
}

func (c Clock) PhaseAt(now time.Time) (Phase, time.Duration) {
	total := c.cfg.DayDuration + c.cfg.NightDuration
	if total <= 0 {
		return PhaseDay, 0
	}
	elapsed := now.Sub(c.cfg.StartAt)
	if elapsed < 0 {
		elapsed = 0
	}
	offset := elapsed % total
	if offset < c.cfg.DayDuration {
		return PhaseDay, c.cfg.DayDuration - offset
	}
	nightOffset := offset - c.cfg.DayDuration
	return PhaseNight, c.cfg.NightDuration - nightOffset
}
