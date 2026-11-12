package main

import (
	"context"
	"errors"

	"github.com/mindsgn-studio/afika-backend/workers/receipt/lib"
)

type paidMark struct {
	ID     string
	TxHash string
	Now    int64
}

type fakeStore struct {
	expired     int64
	expireErr   error
	pending     []lib.Payment
	pendingErr  error
	credits     map[string][]lib.Credit
	creditsErr  error
	creditCalls int
	lastQuery   lib.CreditQuery
	paid        []paidMark
	paidErr     error
}

func (f *fakeStore) ExpirePending(context.Context, int64) (int64, error) {
	return f.expired, f.expireErr
}

func (f *fakeStore) ListPending(context.Context, int64, int) ([]lib.Payment, error) {
	return f.pending, f.pendingErr
}

func (f *fakeStore) ListCredits(_ context.Context, query lib.CreditQuery) ([]lib.Credit, error) {
	f.creditCalls++
	f.lastQuery = query
	if f.creditsErr != nil {
		return nil, f.creditsErr
	}
	return f.credits[query.Address], nil
}

func (f *fakeStore) MarkPaid(_ context.Context, id, txHash string, nowMs int64) error {
	if f.paidErr != nil {
		return f.paidErr
	}
	f.paid = append(f.paid, paidMark{ID: id, TxHash: txHash, Now: nowMs})
	return nil
}

var errStore = errors.New("store failed")
