package calendar_test

import (
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/calendar"
)

func TestCalendar_Invariant15_IndianBankHolidays(t *testing.T) {
	cal := calendar.New(nil)

	// Republic Day 2026: 2026-01-26 (Monday)
	repDay := time.Date(2026, 1, 26, 12, 0, 0, 0, calendar.Location())
	isHoliday, desc := cal.IsBankHoliday(repDay)
	if !isHoliday {
		t.Fatalf("Expected Republic Day (2026-01-26) to be recognized as bank holiday, got false")
	}
	if desc != "Republic Day" {
		t.Errorf("Expected description 'Republic Day', got %q", desc)
	}

	// Next business day after Republic Day should be Tuesday 2026-01-27
	nextBiz := cal.NextBusinessDay(repDay)
	if nextBiz.Day() != 27 || nextBiz.Month() != 1 {
		t.Errorf("Expected next business day to be 2026-01-27, got %s", nextBiz.Format("2006-01-02"))
	}
}

func TestCalendar_SundayAndSecondFourthSaturday(t *testing.T) {
	cal := calendar.New(nil)

	// 2026-02-08 is a Sunday
	sunday := time.Date(2026, 2, 8, 10, 0, 0, 0, calendar.Location())
	isHoliday, desc := cal.IsBankHoliday(sunday)
	if !isHoliday || desc != "Sunday" {
		t.Errorf("Expected Sunday to be recognized, got %v (%s)", isHoliday, desc)
	}

	// 2026-02-14 is 2nd Saturday of February 2026
	secondSat := time.Date(2026, 2, 14, 10, 0, 0, 0, calendar.Location())
	isHoliday, _ = cal.IsBankHoliday(secondSat)
	if !isHoliday {
		t.Errorf("Expected 2nd Saturday (2026-02-14) to be bank holiday, got false")
	}

	// 2026-02-07 is 1st Saturday of February 2026 (Working Saturday for Indian banks)
	firstSat := time.Date(2026, 2, 7, 10, 0, 0, 0, calendar.Location())
	isHoliday, _ = cal.IsBankHoliday(firstSat)
	if isHoliday {
		t.Errorf("Expected 1st Saturday (2026-02-07) to be working day, got holiday")
	}
}

func TestCalendar_AddBusinessDays(t *testing.T) {
	cal := calendar.New(nil)

	// Friday 2026-01-23 + 2 business days:
	// Saturday 24th is 4th Saturday (Holiday)
	// Sunday 25th is Sunday (Holiday)
	// Monday 26th is Republic Day (Holiday)
	// Day 1: Tuesday 27th
	// Day 2: Wednesday 28th
	friday := time.Date(2026, 1, 23, 10, 0, 0, 0, calendar.Location())
	target := cal.AddBusinessDays(friday, 2)

	if target.Day() != 28 || target.Month() != 1 {
		t.Errorf("Expected 2 business days after 2026-01-23 to land on 2026-01-28, got %s", target.Format("2006-01-02"))
	}
}
