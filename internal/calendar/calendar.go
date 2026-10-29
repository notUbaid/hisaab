package calendar

import (
	"fmt"
	"time"
)

// ISTLocationName is the canonical IANA timezone identifier for Indian Standard Time.
const ISTLocationName = "Asia/Kolkata"

var istLocation *time.Location

func init() {
	loc, err := time.LoadLocation(ISTLocationName)
	if err != nil {
		// Fallback fixed zone: UTC+5:30
		loc = time.FixedZone("IST", 5*3600+30*60)
	}
	istLocation = loc
}

// Location returns the Asia/Kolkata time.Location.
func Location() *time.Location {
	return istLocation
}

// Clock provides an injectable time oracle for deterministic testing.
type Clock interface {
	Now() time.Time
}

// SystemClock provides the standard system time in Asia/Kolkata.
type SystemClock struct{}

// Now returns current time in Asia/Kolkata.
func (SystemClock) Now() time.Time {
	return time.Now().In(istLocation)
}

// MockClock provides a controllable clock for tests.
type MockClock struct {
	CurrentTime time.Time
}

// Now returns the mocked time in Asia/Kolkata.
func (m *MockClock) Now() time.Time {
	return m.CurrentTime.In(istLocation)
}

// Calendar manages Indian banking calendar rules and business day calculations.
type Calendar struct {
	clock    Clock
	holidays map[string]string // Key: "YYYY-MM-DD", Value: Description
}

// New creates an Indian business calendar with default system clock.
func New(clock Clock) *Calendar {
	if clock == nil {
		clock = SystemClock{}
	}
	c := &Calendar{
		clock:    clock,
		holidays: make(map[string]string),
	}
	c.seedDefaultHolidays2026()
	return c
}

// AddHoliday registers a specific holiday on the given date (YYYY-MM-DD).
func (c *Calendar) AddHoliday(dateStr, description string) {
	c.holidays[dateStr] = description
}

// IsBankHoliday checks if the given date is a weekend (Sunday, 2nd/4th Saturday) or listed holiday.
func (c *Calendar) IsBankHoliday(t time.Time) (bool, string) {
	tIST := t.In(istLocation)
	weekday := tIST.Weekday()

	if weekday == time.Sunday {
		return true, "Sunday"
	}

	// 2nd and 4th Saturday rule for Indian commercial banks
	if weekday == time.Saturday {
		day := tIST.Day()
		weekNum := (day-1)/7 + 1
		if weekNum == 2 || weekNum == 4 {
			return true, fmt.Sprintf("Bank Saturday (Week %d)", weekNum)
		}
	}

	dateKey := tIST.Format("2006-01-02")
	if desc, ok := c.holidays[dateKey]; ok {
		return true, desc
	}

	return false, ""
}

// NextBusinessDay advances a time until the next operational Indian business day.
func (c *Calendar) NextBusinessDay(t time.Time) time.Time {
	current := t.In(istLocation)
	for {
		isHoliday, _ := c.IsBankHoliday(current)
		if !isHoliday {
			return current
		}
		current = current.AddDate(0, 0, 1)
	}
}

// AddBusinessDays adds n business days to the given time, skipping weekends and holidays.
func (c *Calendar) AddBusinessDays(t time.Time, n int) time.Time {
	current := t.In(istLocation)
	added := 0
	for added < n {
		current = current.AddDate(0, 0, 1)
		isHoliday, _ := c.IsBankHoliday(current)
		if !isHoliday {
			added++
		}
	}
	return current
}

// SettlementCutOffDetermined assigns a settlement batch date based on reception time and cut-off (23:59:59 IST).
func (c *Calendar) SettlementCutOffDetermined(receivedAt time.Time) time.Time {
	tIST := receivedAt.In(istLocation)
	// Normal cut-off is midnight IST
	dateOnly := time.Date(tIST.Year(), tIST.Month(), tIST.Day(), 0, 0, 0, 0, istLocation)
	return c.NextBusinessDay(dateOnly)
}

// seedDefaultHolidays2026 initializes statutory national and banking holidays for 2026.
func (c *Calendar) seedDefaultHolidays2026() {
	c.holidays["2026-01-26"] = "Republic Day"
	c.holidays["2026-03-03"] = "Holi"
	c.holidays["2026-03-21"] = "Id-ul-Fitr (Eid)"
	c.holidays["2026-04-03"] = "Good Friday"
	c.holidays["2026-04-14"] = "Dr. Ambedkar Jayanti"
	c.holidays["2026-05-01"] = "Maharashtra Day / Labour Day"
	c.holidays["2026-05-27"] = "Bakri Eid / Id-ul-Zuha"
	c.holidays["2026-08-15"] = "Independence Day"
	c.holidays["2026-09-04"] = "Janmashtami"
	c.holidays["2026-10-02"] = "Mahatma Gandhi Jayanti"
	c.holidays["2026-10-20"] = "Dussehra"
	c.holidays["2026-11-08"] = "Diwali (Laxmi Pujan)"
	c.holidays["2026-11-24"] = "Guru Nanak Jayanti"
	c.holidays["2026-12-25"] = "Christmas"
}
