package recon_test

import (
	"context"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/recon"
)

func TestMemoryRepository_SaveAndRetrieve(t *testing.T) {
	ctx := context.Background()
	repo := recon.NewMemoryRepository()

	now := time.Now().UTC()
	excs := []recon.ReconException{
		{
			ID:             "EXC-001",
			NetworkOrderID: "ORD-101",
			Class:          recon.ExceptionAmountMismatch,
			ExpectedPaise:  100000,
			ActualPaise:    95000,
			AgeingDays:     2,
			Status:         "OPEN",
			Details:        "Paise discrepancy between buyer and seller claims",
			DetectedAt:     now,
		},
		{
			ID:             "EXC-002",
			NetworkOrderID: "ORD-102",
			Class:          recon.ExceptionMissingCounterpart,
			ExpectedPaise:  50000,
			ActualPaise:    0,
			AgeingDays:     1,
			Status:         "OPEN",
			Details:        "Seller claim missing at settlement cutoff",
			DetectedAt:     now,
		},
	}

	if err := repo.SaveExceptions(ctx, excs); err != nil {
		t.Fatalf("SaveExceptions failed: %v", err)
	}

	// 1. List all exceptions
	all, err := repo.ListExceptions(ctx, "")
	if err != nil {
		t.Fatalf("ListExceptions failed: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 exceptions, got %d", len(all))
	}

	// 2. Get specific exception
	e1, err := repo.GetException(ctx, "EXC-001")
	if err != nil {
		t.Fatalf("GetException failed: %v", err)
	}
	if e1.NetworkOrderID != "ORD-101" || e1.ExpectedPaise != 100000 {
		t.Errorf("unexpected exception fields: %+v", e1)
	}

	// 3. Resolve exception
	if err := repo.ResolveException(ctx, "EXC-001", "Approved tolerance adjustment of 5000 paise"); err != nil {
		t.Fatalf("ResolveException failed: %v", err)
	}

	// 4. Verify status updated to RESOLVED
	e1Updated, err := repo.GetException(ctx, "EXC-001")
	if err != nil {
		t.Fatalf("GetException after resolve failed: %v", err)
	}
	if e1Updated.Status != "RESOLVED" {
		t.Errorf("expected status RESOLVED, got %s", e1Updated.Status)
	}

	// 5. Filter by status
	openOnly, err := repo.ListExceptions(ctx, "OPEN")
	if err != nil {
		t.Fatalf("ListExceptions(OPEN) failed: %v", err)
	}
	if len(openOnly) != 1 || openOnly[0].ID != "EXC-002" {
		t.Errorf("expected 1 OPEN exception (EXC-002), got %d", len(openOnly))
	}
}

func TestMemoryRepository_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := recon.NewMemoryRepository()

	_, err := repo.GetException(ctx, "EXC-NONEXISTENT")
	if err != recon.ErrExceptionNotFound {
		t.Errorf("expected ErrExceptionNotFound, got %v", err)
	}

	err = repo.ResolveException(ctx, "EXC-NONEXISTENT", "some remedy")
	if err != recon.ErrExceptionNotFound {
		t.Errorf("expected ErrExceptionNotFound, got %v", err)
	}
}

func TestPostgresRepository_NilPoolGraceful(t *testing.T) {
	ctx := context.Background()
	repo := recon.NewPostgresRepository(nil)

	if err := repo.SaveExceptions(ctx, nil); err != nil {
		t.Errorf("expected nil error on nil pool save, got %v", err)
	}

	list, err := repo.ListExceptions(ctx, "")
	if err != nil || list != nil {
		t.Errorf("expected nil list and error, got %v, %v", list, err)
	}

	_, err = repo.GetException(ctx, "EXC-001")
	if err != recon.ErrExceptionNotFound {
		t.Errorf("expected ErrExceptionNotFound, got %v", err)
	}

	if err := repo.ResolveException(ctx, "EXC-001", "remedy"); err != recon.ErrExceptionNotFound {
		t.Errorf("expected ErrExceptionNotFound on nil pool, got %v", err)
	}
}
