package groups

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/murongg/SubLane/internal/storage/db"
)

const DefaultID int64 = 1
const MaxGroups = 32

var (
	ErrInput       = errors.New("invalid_group_input")
	ErrNotFound    = errors.New("group_not_found")
	ErrDuplicate   = errors.New("group_exists")
	ErrDefault     = errors.New("default_group_protected")
	ErrLimit       = errors.New("group_limit")
	ErrUnavailable = errors.New("group_unavailable")
)

type Group struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	IsDefault    bool   `json:"is_default"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
	AccountCount int64  `json:"account_count"`
	MemberCount  int64  `json:"member_count"`
}
type Detail struct {
	Group
	AccountIDs []string `json:"account_ids"`
}
type Choice struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
type Input struct {
	Name       string   `json:"name"`
	Enabled    bool     `json:"enabled"`
	AccountIDs []string `json:"account_ids"`
}
type Service struct {
	connection *sql.DB
	queries    *db.Queries
}

func New(connection *sql.DB) *Service {
	return &Service{connection: connection, queries: db.New(connection)}
}

func (s *Service) List(ctx context.Context) ([]Group, error) {
	rows, err := s.queries.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Group, 0, len(rows))
	for _, r := range rows {
		result = append(result, Group{ID: r.ID, Name: r.Name, Enabled: r.Enabled, IsDefault: r.ID == DefaultID, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, AccountCount: r.AccountCount, MemberCount: r.MemberCount})
	}
	return result, nil
}
func (s *Service) Get(ctx context.Context, id int64) (Detail, error) {
	tx, err := s.connection.BeginTx(ctx, nil)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	row, err := q.GetGroup(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	ids, err := q.ListGroupAccounts(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	count, err := q.CountGroupMembers(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	result := Detail{Group: Group{ID: row.ID, Name: row.Name, Enabled: row.Enabled, IsDefault: id == DefaultID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, AccountCount: int64(len(ids)), MemberCount: count}, AccountIDs: ids}
	return result, tx.Commit()
}
func (s *Service) Save(ctx context.Context, id int64, input Input) (Detail, error) {
	name := strings.TrimSpace(input.Name)
	if id < 0 || !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 || input.AccountIDs == nil || len(input.AccountIDs) > 100 {
		return Detail{}, ErrInput
	}
	if id == DefaultID && (!input.Enabled || name != "Default") {
		return Detail{}, ErrDefault
	}
	tx, err := s.connection.BeginTx(ctx, nil)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	if id != 0 {
		if _, err := q.GetGroup(ctx, id); errors.Is(err, sql.ErrNoRows) {
			return Detail{}, ErrNotFound
		} else if err != nil {
			return Detail{}, err
		}
	}
	other, err := q.FindGroupName(ctx, name)
	if err == nil && other != id {
		return Detail{}, ErrDuplicate
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Detail{}, err
	}
	seen := make(map[string]bool, len(input.AccountIDs))
	for _, accountID := range input.AccountIDs {
		if seen[accountID] {
			return Detail{}, ErrInput
		}
		seen[accountID] = true
		exists, err := q.GroupAccountExists(ctx, accountID)
		if err != nil {
			return Detail{}, err
		}
		if !exists {
			return Detail{}, ErrInput
		}
	}
	now := time.Now().Unix()
	if id == 0 {
		count, err := q.CountGroups(ctx)
		if err != nil {
			return Detail{}, err
		}
		if count >= MaxGroups {
			return Detail{}, ErrLimit
		}
		id, err = q.CreateGroup(ctx, db.CreateGroupParams{Name: name, Enabled: input.Enabled, Now: now})
		if err != nil {
			return Detail{}, err
		}
	} else if err := q.UpdateGroup(ctx, db.UpdateGroupParams{ID: id, Name: name, Enabled: input.Enabled, Now: now}); err != nil {
		return Detail{}, err
	}
	// Membership replacement commits atomically: no request can observe a half-edited pool.
	if err := q.ClearGroupAccounts(ctx, id); err != nil {
		return Detail{}, err
	}
	for _, accountID := range input.AccountIDs {
		if err := q.AddGroupAccount(ctx, db.AddGroupAccountParams{GroupID: id, AccountID: accountID}); err != nil {
			return Detail{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Detail{}, err
	}
	return s.Get(ctx, id)
}
func (s *Service) Available(ctx context.Context, userID int64) ([]Choice, error) {
	rows, err := s.queries.ListAvailableGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]Choice, 0, len(rows))
	for _, r := range rows {
		result = append(result, Choice{ID: r.ID, Name: r.Name})
	}
	return result, nil
}
func (s *Service) MemberGroups(ctx context.Context, userID int64) ([]int64, error) {
	if _, err := s.queries.GetMember(ctx, userID); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return s.queries.ListMemberGroups(ctx, userID)
}
func (s *Service) SetMemberGroups(ctx context.Context, userID int64, ids []int64) error {
	if ids == nil || len(ids) > MaxGroups {
		return ErrInput
	}
	tx, err := s.connection.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	if _, err := q.GetMember(ctx, userID); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return ErrInput
		}
		seen[id] = true
		if _, err := q.GetGroup(ctx, id); errors.Is(err, sql.ErrNoRows) {
			return ErrInput
		} else if err != nil {
			return err
		}
	}
	if err := q.ClearMemberGroups(ctx, userID); err != nil {
		return err
	}
	for _, id := range ids {
		if err := q.GrantMemberGroup(ctx, db.GrantMemberGroupParams{GroupID: id, UserID: userID}); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Service) Connection(ctx context.Context, userID int64) (string, error) {
	return s.queries.GroupConnectionStatus(ctx, userID)
}
