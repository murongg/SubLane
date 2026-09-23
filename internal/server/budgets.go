package server

import (
	"errors"
	"net/http"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/gateway"
)

func (h *memberHTTP) ownBudgets(w http.ResponseWriter, r *http.Request) {
	h.readBudgets(w, r, sessionUser(r).ID)
}
func (h *memberHTTP) budgets(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		accountError(w, accounts.ErrInput)
		return
	}
	h.readBudgets(w, r, id)
}
func budgetManagementError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gateway.ErrBudgetLimit):
		writeJSON(w, 409, map[string]string{"error": err.Error()})
	case errors.Is(err, gateway.ErrBudgetSettlement):
		writeJSON(w, 409, map[string]string{"error": err.Error()})
	case errors.Is(err, gateway.ErrTokenAccounting):
		writeJSON(w, 503, map[string]string{"error": err.Error()})
	case errors.Is(err, accounts.ErrInput):
		accountError(w, err)
	default:
		authError(w, err)
	}
}
func (h *memberHTTP) readBudgets(w http.ResponseWriter, r *http.Request, id int64) {
	if !h.available(w) {
		return
	}
	page, err := h.gateway.Budgets(r.Context(), id)
	if err != nil {
		budgetManagementError(w, err)
		return
	}
	writeJSON(w, 200, page)
}
func (h *memberHTTP) saveBudget(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	id, err := pathID(r)
	if err != nil || id <= 0 {
		accountError(w, accounts.ErrInput)
		return
	}
	var input struct {
		GroupID int64  `json:"group_id"`
		Model   string `json:"model"`
		Period  string `json:"period"`
		Limit   int64  `json:"limit"`
		Enabled *bool  `json:"enabled"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Enabled == nil {
		accountError(w, accounts.ErrInput)
		return
	}
	result, err := h.gateway.SaveBudget(r.Context(), id, gateway.BudgetInput{GroupID: input.GroupID, Model: input.Model, Period: input.Period, Limit: input.Limit, Enabled: *input.Enabled})
	if err != nil {
		budgetManagementError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (h *memberHTTP) settleBudget(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	id, err := pathID(r)
	if err != nil || id <= 0 {
		accountError(w, accounts.ErrInput)
		return
	}
	var input struct {
		RequestID string `json:"request_id"`
		Tokens    *int64 `json:"tokens"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Tokens == nil {
		accountError(w, accounts.ErrInput)
		return
	}
	if err := h.gateway.ResolveBudget(r.Context(), id, input.RequestID, *input.Tokens); err != nil {
		budgetManagementError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func budgetErrorDetails(err error) any {
	var budget *gateway.BudgetError
	if errors.As(err, &budget) {
		return struct {
			BudgetID int64  `json:"budget_id"`
			GroupID  int64  `json:"group_id"`
			Model    string `json:"model"`
			Period   string `json:"period"`
			ResetAt  int64  `json:"reset_at"`
		}{budget.BudgetID, budget.GroupID, budget.Model, budget.Period, budget.ResetAt}
	}
	return nil
}
