package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/murongg/SubLane/internal/storage/db"
)

var (
	ErrUsernameTaken  = errors.New("username_taken")
	ErrMemberNotFound = errors.New("member_not_found")
)

type Member struct {
	User
	Enabled   bool  `json:"enabled"`
	CreatedAt int64 `json:"created_at"`
}

type MemberPage struct {
	Members    []Member `json:"members"`
	NextCursor int64    `json:"next_cursor"`
}

func (s *Service) ListMembers(ctx context.Context, beforeID int64) (MemberPage, error) {
	page := MemberPage{Members: []Member{}}
	if beforeID < 0 {
		return page, ErrInput
	}
	rows, err := s.queries.ListMembers(ctx, beforeID)
	if err != nil {
		return page, err
	}
	for _, row := range rows {
		page.Members = append(page.Members, Member{User: User{ID: row.ID, Username: row.Username, Role: Role(row.Role)}, Enabled: row.Enabled, CreatedAt: row.CreatedAt})
	}
	if len(page.Members) > 50 {
		page.Members = page.Members[:50]
		page.NextCursor = page.Members[49].ID
	}
	return page, nil
}

func (s *Service) CreateMember(ctx context.Context, username, password string) (Member, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !validCredentials(username, password, 20) {
		return Member{}, ErrInput
	}
	if err := s.acquire(ctx); err != nil {
		return Member{}, err
	}
	defer func() { <-s.hashSlots }()
	hash := hashPassword(password)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Member{}, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	row, err := queries.CreateMember(ctx, db.CreateMemberParams{Username: username, PasswordHash: hash, CreatedAt: s.now().Unix()})
	if errors.Is(err, sql.ErrNoRows) {
		return Member{}, ErrUsernameTaken
	}
	if err != nil {
		return Member{}, err
	}
	// A newly created member keeps the existing shared-pool behavior until an administrator changes grants.
	if err := queries.AddDefaultGroupMember(ctx, row.ID); err != nil {
		return Member{}, err
	}
	if err := tx.Commit(); err != nil {
		return Member{}, err
	}
	return Member{User: User{ID: row.ID, Username: row.Username, Role: Role(row.Role)}, Enabled: row.Enabled, CreatedAt: row.CreatedAt}, nil
}

func (s *Service) SetMemberEnabled(ctx context.Context, id int64, enabled bool) (Member, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Member{}, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	row, err := queries.GetMember(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Member{}, ErrMemberNotFound
	}
	if err != nil {
		return Member{}, err
	}
	if err := queries.SetMemberEnabled(ctx, db.SetMemberEnabledParams{Enabled: enabled, ID: id}); err != nil {
		return Member{}, err
	}
	// Revocation and account status commit together, and re-enabling never revives old cookies.
	if !enabled {
		if err := queries.DeleteUserSessions(ctx, id); err != nil {
			return Member{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Member{}, err
	}
	return Member{User: User{ID: row.ID, Username: row.Username, Role: Role(row.Role)}, Enabled: enabled, CreatedAt: row.CreatedAt}, nil
}
