package ledger_test

import (
	"context"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/ledger"
)

func TestMemoryStore_Lifecycle(t *testing.T) {
	ctx := context.Background()
	store := ledger.NewMemoryStore(nil)

	// 1. Register accounts
	err := store.RegisterAccount(ctx, "collector:ondc_settlement", ledger.AccountLiability, "INR", true)
	if err != nil {
		t.Fatalf("RegisterAccount failed: %v", err)
	}
	err = store.RegisterAccount(ctx, "ext:nodal_clearing", ledger.AccountExternal, "INR", false)
	if err != nil {
		t.Fatalf("RegisterAccount failed: %v", err)
	}

	// 2. Get Account
	acc, err := store.GetAccount(ctx, "collector:ondc_settlement")
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acc.Name != "collector:ondc_settlement" || !acc.Constrained {
		t.Errorf("unexpected account: %+v", acc)
	}

	// 3. Post transaction
	req := ledger.PostRequest{
		IdempotencyKey: "store-tx-001",
		CauseType:      ledger.CauseInstruction,
		CauseID:        1,
		Description:    "Store test transaction",
		Lines: []ledger.EntryLine{
			{AccountName: "collector:ondc_settlement", Direction: ledger.Credit, AmountPaise: 100000},
			{AccountName: "ext:nodal_clearing", Direction: ledger.Debit, AmountPaise: 100000},
		},
		PostedAt: time.Now(),
	}

	res, err := store.Post(ctx, req)
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}
	if res.TotalPaise != 100000 {
		t.Errorf("expected 100000 total paise, got %d", res.TotalPaise)
	}

	// 4. Check balances
	bal, err := store.GetBalance(ctx, "collector:ondc_settlement")
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}
	if bal != 100000 {
		t.Errorf("expected 100000 balance, got %d", bal)
	}

	// 5. List accounts
	list, err := store.ListAccounts(ctx)
	if err != nil || len(list) != 2 {
		t.Errorf("expected 2 accounts, got %d, err %v", len(list), err)
	}
}

func TestSQLStore_NilPoolGraceful(t *testing.T) {
	ctx := context.Background()
	store := ledger.NewSQLStore(nil)

	_, err := store.GetAccount(ctx, "collector:ondc_settlement")
	if err != ledger.ErrAccountNotFound {
		t.Errorf("expected ErrAccountNotFound on nil pool, got %v", err)
	}

	list, err := store.ListAccounts(ctx)
	if err != nil || list != nil {
		t.Errorf("expected nil list, nil err on nil pool, got %v, %v", list, err)
	}

	_, err = store.GetBalance(ctx, "collector:ondc_settlement")
	if err == nil {
		t.Error("expected error on nil pool GetBalance, got nil")
	}

	err = store.RegisterAccount(ctx, "test", ledger.AccountAsset, "INR", false)
	if err == nil {
		t.Error("expected error on nil pool RegisterAccount, got nil")
	}

	_, err = store.Post(ctx, ledger.PostRequest{})
	if err == nil {
		t.Error("expected error on nil pool Post, got nil")
	}
}
