package service

import (
	"testing"

	"github.com/tigerowo/freedom/model"
)

// 表驱动：复用 hold 时字段必须完全一致；任一不一致即返回 error（不允许"金额不同但同 requestID"）。
func TestAssertHoldMatchesRequiresExactFields(t *testing.T) {
	base := &model.BalanceHold{
		ID:        "hold-1",
		UserID:    "user-A",
		RequestID: "req-1",
		Amount:    100,
		Model:     "gpt-image-1",
		Path:      "/images/generations",
		Status:    model.BalanceHoldHeld,
	}

	cases := []struct {
		name      string
		mutate    func(h *model.BalanceHold)
		expectErr bool
	}{
		{name: "完全一致", mutate: func(h *model.BalanceHold) {}, expectErr: false},
		{name: "userID 不一致", mutate: func(h *model.BalanceHold) { h.UserID = "user-B" }, expectErr: true},
		{name: "requestID 不一致", mutate: func(h *model.BalanceHold) { h.RequestID = "req-2" }, expectErr: true},
		{name: "amount 不一致", mutate: func(h *model.BalanceHold) { h.Amount = 1 }, expectErr: true},
		{name: "model 不一致", mutate: func(h *model.BalanceHold) { h.Model = "grok-imagine" }, expectErr: true},
		{name: "path 不一致", mutate: func(h *model.BalanceHold) { h.Path = "/chat/completions" }, expectErr: true},
		{name: "status=settled", mutate: func(h *model.BalanceHold) { h.Status = model.BalanceHoldSettled }, expectErr: true},
		{name: "status=cancelled", mutate: func(h *model.BalanceHold) { h.Status = model.BalanceHoldCancelled }, expectErr: true},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			hold := *base
			c.mutate(&hold)
			err := assertHoldMatches(&hold, "user-A", "gpt-image-1", 100, "/images/generations", "req-1")
			gotErr := err != nil
			if gotErr != c.expectErr {
				t.Errorf("assertHoldMatches err=%v, want err=%v", err, c.expectErr)
			}
		})
	}
}

// nil 指针防御：nil hold 永远 error。
func TestAssertHoldMatchesNil(t *testing.T) {
	if err := assertHoldMatches(nil, "user", "model", 1, "/path", "req"); err == nil {
		t.Errorf("assertHoldMatches(nil) must return error, got nil")
	}
}
