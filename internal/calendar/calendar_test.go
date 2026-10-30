package calendar_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/calendar"
)

func TestCalendar_Invariant15_IndianBankHolidays_2026(t *testing.T) {
	cal := calendar.New(nil)

	// Republic Day 2026: 2026-01-26 (Monday)
	repDay := time.Date(2026, 1, 26, 12, 0, 0, 0, calendar.Location())
	isHoliday, desc, err := cal.IsBankHoliday(repDay)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !isHoliday {
		t.Fatalf("Expected Republic Day (2026-01-26) to be recognized as bank holiday, got false")
	}
	if desc != "Republic Day" {
		t.Errorf("Expected description 'Republic Day', got %q", desc)
	}

	// Next business day after Republic Day should be Tuesday 2026-01-27
	nextBiz, err := cal.NextBusinessDay(repDay)
	if err != nil {
		t.Fatalf("NextBusinessDay failed: %v", err)
	}
	if nextBiz.Day() != 27 || nextBiz.Month() != 1 {
		t.Errorf("Expected next business day to be 2026-01-27, got %s", nextBiz.Format("2006-01-02"))
	}
}

func TestCalendar_StatutoryHolidays_2027(t *testing.T) {
	cal := calendar.New(nil)

	// Republic Day 2027: 2027-01-26 (Tuesday)
	repDay2027 := time.Date(2027, 1, 26, 12, 0, 0, 0, calendar.Location())
	isHoliday, desc, err := cal.IsBankHoliday(repDay2027)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !isHoliday || desc != "Republic Day" {
		t.Errorf("Expected 2027 Republic Day recognized, got %v (%s)", isHoliday, desc)
	}

	nextBiz, err := cal.NextBusinessDay(repDay2027)
	if err != nil {
		t.Fatalf("NextBusinessDay failed: %v", err)
	}
	if nextBiz.Day() != 27 || nextBiz.Month() != 1 {
		t.Errorf("Expected next business day to be 2027-01-27, got %s", nextBiz.Format("2006-01-02"))
	}
}

func TestCalendar_FailClosed_UnconfiguredYear(t *testing.T) {
	cal := calendar.New(nil)

	// Year 2030 is unconfigured
	t2030 := time.Date(2030, 5, 1, 10, 0, 0, 0, calendar.Location())
	_, _, err := cal.IsBankHoliday(t2030)
	if !errors.Is(err, calendar.ErrYearNotConfigured) {
		t.Fatalf("Expected ErrYearNotConfigured for year 2030, got %v", err)
	}

	_, err = cal.NextBusinessDay(t2030)
	if !errors.Is(err, calendar.ErrYearNotConfigured) {
		t.Fatalf("Expected ErrYearNotConfigured from NextBusinessDay for 2030, got %v", err)
	}

	_, err = cal.SettlementCutOff(t2030)
	if !errors.Is(err, calendar.ErrYearNotConfigured) {
		t.Fatalf("Expected ErrYearNotConfigured from SettlementCutOff for 2030, got %v", err)
	}
}

func TestCalendar_SundayAndSecondFourthSaturday(t *testing.T) {
	cal := calendar.New(nil)

	// 2026-02-08 is a Sunday
	sunday := time.Date(2026, 2, 8, 10, 0, 0, 0, calendar.Location())
	isHoliday, desc, err := cal.IsBankHoliday(sunday)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !isHoliday || desc != "Sunday" {
		t.Errorf("Expected Sunday to be recognized, got %v (%s)", isHoliday, desc)
	}

	// 2026-02-14 is 2nd Saturday of February 2026
	secondSat := time.Date(2026, 2, 14, 10, 0, 0, 0, calendar.Location())
	isHoliday, _, _ = cal.IsBankHoliday(secondSat)
	if !isHoliday {
		t.Errorf("Expected 2nd Saturday (2026-02-14) to be bank holiday, got false")
	}

	// 2026-02-07 is 1st Saturday of February 2026 (Working Saturday for Indian banks)
	firstSat := time.Date(2026, 2, 7, 10, 0, 0, 0, calendar.Location())
	isHoliday, _, _ = cal.IsBankHoliday(firstSat)
	if isHoliday {
		t.Errorf("Expected 1st Saturday (2026-02-07) to be working day, got holiday")
	}
}

func TestCalendar_CutOffBoundary_TMinus1s_T_TPlus1s(t *testing.T) {
	cal := calendar.New(nil)
	cal.SetCutOffTime(23, 59, 59)

	loc := calendar.Location()
	// Wednesday 2026-10-28 is a regular business day
	// Cut-off boundary is 23:59:59 IST

	// T - 1s (23:59:58): stays in 2026-10-28
	tMinus1s := time.Date(2026, 10, 28, 23, 59, 58, 0, loc)
	resMinus, err := cal.SettlementCutOff(tMinus1s)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resMinus.Format("2006-01-02") != "2026-10-28" {
		t.Errorf("T-1s expected 2026-10-28, got %s", resMinus.Format("2006-01-02"))
	}

	// T (23:59:59): exact cut-off rolls to tomorrow (2026-10-29)
	tCutOff := time.Date(2026, 10, 28, 23, 59, 59, 0, loc)
	resCutOff, err := cal.SettlementCutOff(tCutOff)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resCutOff.Format("2006-01-02") != "2026-10-29" {
		t.Errorf("T exact expected 2026-10-29, got %s", resCutOff.Format("2006-01-02"))
	}

	// T + 1s (next second, 2026-10-29 00:00:00): rolls to 2026-10-29
	tPlus1s := time.Date(2026, 10, 29, 0, 0, 0, 0, loc)
	resPlus, err := cal.SettlementCutOff(tPlus1s)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resPlus.Format("2006-01-02") != "2026-10-29" {
		t.Errorf("T+1s expected 2026-10-29, got %s", resPlus.Format("2006-01-02"))
	}

	// Friday at cut-off rolls over Saturday & Sunday to Monday
	// 2026-10-30 is Friday. At 23:59:59, rolls to 2026-10-31 (Saturday).
	// Saturday 31st is 5th Saturday (working) unless holiday; Sunday Nov 1 is holiday.
	fridayCutOff := time.Date(2026, 10, 23, 23, 59, 59, 0, loc)
	// Oct 23 is Friday. Next day is 4th Saturday (Oct 24, bank holiday), Oct 25 is Sunday (holiday),
	// Oct 26 is holiday (if configured) or Monday Oct 26.
	resFri, err := cal.SettlementCutOff(fridayCutOff)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resFri.Weekday() == time.Saturday || resFri.Weekday() == time.Sunday {
		t.Errorf("Settlement date cannot land on weekend, got %s", resFri.Weekday())
	}
}

func TestCalendar_ConcurrencySafety(t *testing.T) {
	cal := calendar.New(nil)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(2)
		// Reader goroutine
		go func(idx int) {
			defer wg.Done()
			d := time.Date(2026, 1, 26, 10, 0, 0, 0, calendar.Location())
			_, _, _ = cal.IsBankHoliday(d)
			_ = cal.SettlementCutOffDetermined(d)
		}(i)

		// Writer goroutine
		go func(idx int) {
			defer wg.Done()
			cal.AddHoliday("2028-01-01", "New Year 2028", "https://rbi.org.in")
		}(i)
	}
	wg.Wait()
}
