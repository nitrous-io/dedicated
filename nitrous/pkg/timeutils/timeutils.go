package timeutils

import "time"

func FirstOfNextMonth(curTime time.Time) time.Time {
	y := curTime.Year()
	m := curTime.Month()
	nm := m + 1
	if nm%12 == 1 {
		y = y + 1
		nm = 1
	}
	return time.Date(y, nm, 1, 0, 0, 0, 0, time.UTC)
}
