package scenariodsl_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ubaid/hisaab/internal/scenariodsl"
)

func findScenarioPath(t *testing.T, filename string) string {
	candidates := []string{
		filepath.Join("spec", "scenarios", filename),
		filepath.Join("..", "..", "spec", "scenarios", filename),
		filepath.Join(".", "spec", "scenarios", filename),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	dir, err := os.Getwd()
	if err == nil {
		for i := 0; i < 5; i++ {
			c := filepath.Join(dir, "spec", "scenarios", filename)
			if _, err := os.Stat(c); err == nil {
				return c
			}
			dir = filepath.Dir(dir)
		}
	}
	t.Fatalf("could not locate scenario file %s in candidate paths", filename)
	return ""
}

func TestRunner_EC01_MissingCounterpart(t *testing.T) {
	path := findScenarioPath(t, "ec-01.yaml")
	sc, err := scenariodsl.LoadScenarioFile(path)
	if err != nil {
		t.Fatalf("failed to load ec-01.yaml: %v", err)
	}

	runner := scenariodsl.NewRunner()
	res, err := runner.Run(context.Background(), sc)
	if err != nil {
		t.Fatalf("scenario EC-01 failed: %v", err)
	}

	if !res.Passed {
		t.Fatalf("expected scenario to pass, got failed")
	}
	if res.NetPayoutPaise != 0 {
		t.Fatalf("expected 0 net payout on missing counterpart, got %d", res.NetPayoutPaise)
	}
	if res.FinalBalance != 2000000 {
		t.Fatalf("expected final balance 2000000 paise, got %d", res.FinalBalance)
	}
}

func TestRunner_EC03_IdenticalMatchAndSettle(t *testing.T) {
	path := findScenarioPath(t, "ec-03.yaml")
	sc, err := scenariodsl.LoadScenarioFile(path)
	if err != nil {
		t.Fatalf("failed to load ec-03.yaml: %v", err)
	}

	runner := scenariodsl.NewRunner()
	res, err := runner.Run(context.Background(), sc)
	if err != nil {
		t.Fatalf("scenario EC-03 failed: %v", err)
	}

	if !res.Passed {
		t.Fatalf("expected scenario to pass, got failed")
	}
	if res.NetPayoutPaise != 396000 {
		t.Fatalf("expected 396000 net payout, got %d", res.NetPayoutPaise)
	}
	if res.FinalBalance != 4604000 {
		t.Fatalf("expected final balance 4604000 paise, got %d", res.FinalBalance)
	}
	if len(res.InvariantsVerified) < 4 {
		t.Fatalf("expected at least 4 invariants verified, got %d", len(res.InvariantsVerified))
	}
}

func TestRunner_EC04_PartialOverlap(t *testing.T) {
	path := findScenarioPath(t, "ec-04.yaml")
	sc, err := scenariodsl.LoadScenarioFile(path)
	if err != nil {
		t.Fatalf("failed to load ec-04.yaml: %v", err)
	}

	runner := scenariodsl.NewRunner()
	res, err := runner.Run(context.Background(), sc)
	if err != nil {
		t.Fatalf("scenario EC-04 failed: %v", err)
	}

	if !res.Passed {
		t.Fatalf("expected scenario to pass, got failed")
	}
	if res.NetPayoutPaise != 198000 {
		t.Fatalf("expected 198000 net payout, got %d", res.NetPayoutPaise)
	}
	if res.FinalBalance != 4802000 {
		t.Fatalf("expected final balance 4802000 paise, got %d", res.FinalBalance)
	}
}

func TestRunner_EC12_InsufficientBalance(t *testing.T) {
	path := findScenarioPath(t, "ec-12.yaml")
	sc, err := scenariodsl.LoadScenarioFile(path)
	if err != nil {
		t.Fatalf("failed to load ec-12.yaml: %v", err)
	}

	runner := scenariodsl.NewRunner()
	res, err := runner.Run(context.Background(), sc)
	if err != nil {
		t.Fatalf("scenario EC-12 failed: %v", err)
	}

	if !res.Passed {
		t.Fatalf("expected scenario to pass, got failed")
	}
	if res.NetPayoutPaise != 0 {
		t.Fatalf("expected 0 net payout on insufficient balance, got %d", res.NetPayoutPaise)
	}
	if res.FinalBalance != 50000 {
		t.Fatalf("expected balance intact at 50000 paise, got %d", res.FinalBalance)
	}
}

func TestRunner_LoadInvalidYAML(t *testing.T) {
	invalidYAML := []byte("invalid: [yaml: unclosed")
	_, err := scenariodsl.LoadScenarioYAML(invalidYAML)
	if err == nil {
		t.Fatalf("expected error loading invalid yaml, got nil")
	}

	missingID := []byte("title: Missing ID")
	_, err = scenariodsl.LoadScenarioYAML(missingID)
	if err == nil {
		t.Fatalf("expected error for missing ID, got nil")
	}
}
