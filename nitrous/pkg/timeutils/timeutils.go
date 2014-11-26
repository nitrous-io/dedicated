package timeutils

import "time"

func FirstOfNextMonth() time.Time {
	y := time.Now().Year()
	m := time.Now().Month()
	nm := m + 1
	if nm%12 == 1 {
		y = y + 1
		nm = 1
	}
	return time.Date(y, nm, 1, 0, 0, 0, 0, time.UTC)
}
