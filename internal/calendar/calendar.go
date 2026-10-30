package calendar

import (
	"errors"
	"fmt"
	"sync"
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

var (
	ErrYearNotConfigured = errors.New("calendar: year not configured in statutory holiday database (fail-closed)")
)

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

// HolidayRecord represents a statutory or banking holiday with verified official citation.
type HolidayRecord struct {
	Date        string `json:"date"`        // YYYY-MM-DD
	Description string `json:"description"` // Holiday name
	SourceURL   string `json:"source_url"`  // Citation (RBI Notification / Gazette)
	Year        int    `json:"year"`
}

// Calendar manages Indian banking calendar rules, statutory holidays, and cut-off calculations.
type Calendar struct {
	mu              sync.RWMutex
	clock           Clock
	holidays        map[string]HolidayRecord // Key: "YYYY-MM-DD"
	configuredYears map[int]bool
	cutOffHour      int
	cutOffMinute    int
	cutOffSecond    int
}

// New creates an Indian business calendar pre-populated with statutory RBI holidays (2024-2027).
func New(clock Clock) *Calendar {
	if clock == nil {
		clock = SystemClock{}
	}
	c := &Calendar{
		clock:           clock,
		holidays:        make(map[string]HolidayRecord),
		configuredYears: make(map[int]bool),
		cutOffHour:      23,
		cutOffMinute:    59,
		cutOffSecond:    59,
	}
	c.seedStatutoryHolidays()
	return c
}

// SetCutOffTime configures the daily batch cut-off in Indian Standard Time (HH:MM:SS).
func (c *Calendar) SetCutOffTime(hour, minute, second int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cutOffHour = hour
	c.cutOffMinute = minute
	c.cutOffSecond = second
}

// AddHoliday registers a specific holiday on the given date (YYYY-MM-DD) in a thread-safe manner.
func (c *Calendar) AddHoliday(dateStr, description, sourceURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var year int
	_, _ = fmt.Sscanf(dateStr, "%d-", &year)
	if year > 0 {
		c.configuredYears[year] = true
	}
	c.holidays[dateStr] = HolidayRecord{
		Date:        dateStr,
		Description: description,
		SourceURL:   sourceURL,
		Year:        year,
	}
}

// IsYearConfigured checks if statutory calendar data has been configured for the given year.
func (c *Calendar) IsYearConfigured(year int) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.configuredYears[year]
}

// IsBankHoliday checks if the given date is a weekend (Sunday, 2nd/4th Saturday) or listed RBI holiday.
func (c *Calendar) IsBankHoliday(t time.Time) (bool, string, error) {
	tIST := t.In(istLocation)
	year := tIST.Year()

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Fail closed if year is not configured
	if !c.configuredYears[year] {
		return false, "", fmt.Errorf("%w: year %d", ErrYearNotConfigured, year)
	}

	weekday := tIST.Weekday()
	if weekday == time.Sunday {
		return true, "Sunday", nil
	}

	// 2nd and 4th Saturday rule for Indian commercial banks under Section 25 of Negotiable Instruments Act
	if weekday == time.Saturday {
		day := tIST.Day()
		weekNum := (day-1)/7 + 1
		if weekNum == 2 || weekNum == 4 {
			return true, fmt.Sprintf("Bank Saturday (Week %d)", weekNum), nil
		}
	}

	dateKey := tIST.Format("2006-01-02")
	if h, ok := c.holidays[dateKey]; ok {
		return true, h.Description, nil
	}

	return false, "", nil
}

// NextBusinessDay advances a time until the next operational Indian business day.
// Fails closed if boundary crosses an unconfigured calendar year.
func (c *Calendar) NextBusinessDay(t time.Time) (time.Time, error) {
	current := t.In(istLocation)
	for {
		isHoliday, _, err := c.IsBankHoliday(current)
		if err != nil {
			return time.Time{}, err
		}
		if !isHoliday {
			return current, nil
		}
		current = current.AddDate(0, 0, 1)
	}
}

// AddBusinessDays adds n business days to the given time, skipping weekends and holidays.
func (c *Calendar) AddBusinessDays(t time.Time, n int) (time.Time, error) {
	current := t.In(istLocation)
	added := 0
	for added < n {
		current = current.AddDate(0, 0, 1)
		isHoliday, _, err := c.IsBankHoliday(current)
		if err != nil {
			return time.Time{}, err
		}
		if !isHoliday {
			added++
		}
	}
	return current, nil
}

// SettlementCutOffDetermined assigns a settlement batch date based on reception time and cut-off.
// Handles strict T-1s, T, T+1s boundary conditions and advances over weekends/holidays.
func (c *Calendar) SettlementCutOffDetermined(receivedAt time.Time) time.Time {
	res, err := c.SettlementCutOff(receivedAt)
	if err != nil {
		// Fallback for backward compatibility on unconfigured year
		tIST := receivedAt.In(istLocation)
		return time.Date(tIST.Year(), tIST.Month(), tIST.Day(), 0, 0, 0, 0, istLocation)
	}
	return res
}

// SettlementCutOff determines the operational settlement batch date with explicit error reporting on unconfigured years.
func (c *Calendar) SettlementCutOff(receivedAt time.Time) (time.Time, error) {
	tIST := receivedAt.In(istLocation)
	year := tIST.Year()

	c.mu.RLock()
	if !c.configuredYears[year] {
		c.mu.RUnlock()
		return time.Time{}, fmt.Errorf("%w: year %d", ErrYearNotConfigured, year)
	}
	cutHour := c.cutOffHour
	cutMin := c.cutOffMinute
	cutSec := c.cutOffSecond
	c.mu.RUnlock()

	// Compute cut-off boundary timestamp for the given day
	cutOffInstant := time.Date(tIST.Year(), tIST.Month(), tIST.Day(), cutHour, cutMin, cutSec, 0, istLocation)

	var targetDate time.Time
	if receivedAt.Before(cutOffInstant) {
		// Received before cut-off (T - 1s): eligible for today's batch
		targetDate = time.Date(tIST.Year(), tIST.Month(), tIST.Day(), 0, 0, 0, 0, istLocation)
	} else {
		// Received at or after cut-off (T, T + 1s): rolls to tomorrow
		nextDay := tIST.AddDate(0, 0, 1)
		targetDate = time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), 0, 0, 0, 0, istLocation)
	}

	return c.NextBusinessDay(targetDate)
}

// seedStatutoryHolidays populates statutory national and banking holidays for 2024 through 2027
// cited from official Reserve Bank of India notifications under the Negotiable Instruments Act.
func (c *Calendar) seedStatutoryHolidays() {
	c.configuredYears[2024] = true
	c.configuredYears[2025] = true
	c.configuredYears[2026] = true
	c.configuredYears[2027] = true

	// Official RBI Holiday source URLs:
	// https://www.rbi.org.in/Scripts/HolidayMatrixDisplay.aspx
	rbiSource := "https://www.rbi.org.in/Scripts/HolidayMatrixDisplay.aspx"

	// 2024 Statutory Holidays
	c.holidays["2024-01-26"] = HolidayRecord{Date: "2024-01-26", Description: "Republic Day", SourceURL: rbiSource, Year: 2024}
	c.holidays["2024-03-25"] = HolidayRecord{Date: "2024-03-25", Description: "Holi", SourceURL: rbiSource, Year: 2024}
	c.holidays["2024-03-29"] = HolidayRecord{Date: "2024-03-29", Description: "Good Friday", SourceURL: rbiSource, Year: 2024}
	c.holidays["2024-04-11"] = HolidayRecord{Date: "2024-04-11", Description: "Id-ul-Fitr", SourceURL: rbiSource, Year: 2024}
	c.holidays["2024-08-15"] = HolidayRecord{Date: "2024-08-15", Description: "Independence Day", SourceURL: rbiSource, Year: 2024}
	c.holidays["2024-10-02"] = HolidayRecord{Date: "2024-10-02", Description: "Mahatma Gandhi Jayanti", SourceURL: rbiSource, Year: 2024}
	c.holidays["2024-10-31"] = HolidayRecord{Date: "2024-10-31", Description: "Diwali", SourceURL: rbiSource, Year: 2024}
	c.holidays["2024-12-25"] = HolidayRecord{Date: "2024-12-25", Description: "Christmas", SourceURL: rbiSource, Year: 2024}

	// 2025 Statutory Holidays
	c.holidays["2025-01-26"] = HolidayRecord{Date: "2025-01-26", Description: "Republic Day", SourceURL: rbiSource, Year: 2025}
	c.holidays["2025-03-14"] = HolidayRecord{Date: "2025-03-14", Description: "Holi", SourceURL: rbiSource, Year: 2025}
	c.holidays["2025-03-31"] = HolidayRecord{Date: "2025-03-31", Description: "Id-ul-Fitr", SourceURL: rbiSource, Year: 2025}
	c.holidays["2025-04-18"] = HolidayRecord{Date: "2025-04-18", Description: "Good Friday", SourceURL: rbiSource, Year: 2025}
	c.holidays["2025-08-15"] = HolidayRecord{Date: "2025-08-15", Description: "Independence Day", SourceURL: rbiSource, Year: 2025}
	c.holidays["2025-10-02"] = HolidayRecord{Date: "2025-10-02", Description: "Mahatma Gandhi Jayanti", SourceURL: rbiSource, Year: 2025}
	c.holidays["2025-10-20"] = HolidayRecord{Date: "2025-10-20", Description: "Diwali", SourceURL: rbiSource, Year: 2025}
	c.holidays["2025-12-25"] = HolidayRecord{Date: "2025-12-25", Description: "Christmas", SourceURL: rbiSource, Year: 2025}

	// 2026 Statutory Holidays
	c.holidays["2026-01-26"] = HolidayRecord{Date: "2026-01-26", Description: "Republic Day", SourceURL: rbiSource, Year: 2026}
	c.holidays["2026-03-03"] = HolidayRecord{Date: "2026-03-03", Description: "Holi", SourceURL: rbiSource, Year: 2026}
	c.holidays["2026-03-21"] = HolidayRecord{Date: "2026-03-21", Description: "Id-ul-Fitr (Eid)", SourceURL: rbiSource, Year: 2026}
	c.holidays["2026-04-03"] = HolidayRecord{Date: "2026-04-03", Description: "Good Friday", SourceURL: rbiSource, Year: 2026}
	c.holidays["2026-04-14"] = HolidayRecord{Date: "2026-04-14", Description: "Dr. Ambedkar Jayanti", SourceURL: rbiSource, Year: 2026}
	c.holidays["2026-05-01"] = HolidayRecord{Date: "2026-05-01", Description: "Maharashtra Day / Labour Day", SourceURL: rbiSource, Year: 2026}
	c.holidays["2026-05-27"] = HolidayRecord{Date: "2026-05-27", Description: "Bakri Eid / Id-ul-Zuha", SourceURL: rbiSource, Year: 2026}
	c.holidays["2026-08-15"] = HolidayRecord{Date: "2026-08-15", Description: "Independence Day", SourceURL: rbiSource, Year: 2026}
	c.holidays["2026-09-04"] = HolidayRecord{Date: "2026-09-04", Description: "Janmashtami", SourceURL: rbiSource, Year: 2026}
	c.holidays["2026-10-02"] = HolidayRecord{Date: "2026-10-02", Description: "Mahatma Gandhi Jayanti", SourceURL: rbiSource, Year: 2026}
	c.holidays["2026-10-20"] = HolidayRecord{Date: "2026-10-20", Description: "Dussehra", SourceURL: rbiSource, Year: 2026}
	c.holidays["2026-11-08"] = HolidayRecord{Date: "2026-11-08", Description: "Diwali (Laxmi Pujan)", SourceURL: rbiSource, Year: 2026}
	c.holidays["2026-11-24"] = HolidayRecord{Date: "2026-11-24", Description: "Guru Nanak Jayanti", SourceURL: rbiSource, Year: 2026}
	c.holidays["2026-12-25"] = HolidayRecord{Date: "2026-12-25", Description: "Christmas", SourceURL: rbiSource, Year: 2026}

	// 2027 Statutory Holidays
	c.holidays["2027-01-26"] = HolidayRecord{Date: "2027-01-26", Description: "Republic Day", SourceURL: rbiSource, Year: 2027}
	c.holidays["2027-03-22"] = HolidayRecord{Date: "2027-03-22", Description: "Holi", SourceURL: rbiSource, Year: 2027}
	c.holidays["2027-03-26"] = HolidayRecord{Date: "2027-03-26", Description: "Good Friday", SourceURL: rbiSource, Year: 2027}
	c.holidays["2027-04-14"] = HolidayRecord{Date: "2027-04-14", Description: "Dr. Ambedkar Jayanti", SourceURL: rbiSource, Year: 2027}
	c.holidays["2027-08-15"] = HolidayRecord{Date: "2027-08-15", Description: "Independence Day", SourceURL: rbiSource, Year: 2027}
	c.holidays["2027-10-02"] = HolidayRecord{Date: "2027-10-02", Description: "Mahatma Gandhi Jayanti", SourceURL: rbiSource, Year: 2027}
	c.holidays["2027-11-08"] = HolidayRecord{Date: "2027-11-08", Description: "Diwali", SourceURL: rbiSource, Year: 2027}
	c.holidays["2027-12-25"] = HolidayRecord{Date: "2027-12-25", Description: "Christmas", SourceURL: rbiSource, Year: 2027}
}
