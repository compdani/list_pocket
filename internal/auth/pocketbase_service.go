package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	pbcore "github.com/pocketbase/pocketbase/core"
)

// PocketBaseAuthService wraps auth-record operations used by the auth module.
// It centralizes direct PocketBase interactions so login and token flows
// can migrate incrementally without scattering auth-record logic.
type PocketBaseAuthService struct {
	a *Auth
}

func newPocketBaseAuthService(a *Auth) *PocketBaseAuthService {
	return &PocketBaseAuthService{a: a}
}

// FindByUsername returns an auth record by username.
func (s *PocketBaseAuthService) FindByUsername(username string) (*pbcore.Record, error) {
	return s.a.findAuthRecordByUsername(username)
}

// FindByUserID returns an auth record mapped to the given legacy user ID.
func (s *PocketBaseAuthService) FindByUserID(userID int) (*pbcore.Record, error) {
	return s.a.findAuthRecordByUserID(userID)
}

// UpsertUser stores credentials and user status in PocketBase auth records first.
func (s *PocketBaseAuthService) UpsertUser(u User, lookupUsername string) (*pbcore.Record, error) {
	rec, err := s.lookupRecord(u, lookupUsername)
	if err != nil {
		return nil, err
	}

	email := strings.TrimSpace(u.Email.String)
	if email == "" {
		email = fmt.Sprintf("%s@api.local", strings.ToLower(u.Username))
	}

	rec.SetEmail(email)
	rec.Set("username", u.Username)
	rec.Set("legacy_user_id", u.ID)
	rec.Set("user_type", u.Type)
	rec.Set("status", u.Status)
	rec.Set("role", strconv.Itoa(u.UserRoleID))
	rec.SetVerified(true)

	if u.Password.String != "" {
		if regexBcryptHash.MatchString(u.Password.String) {
			rec.SetRaw(pbcore.FieldNamePassword, u.Password.String)
		} else {
			rec.Set(pbcore.FieldNamePassword, u.Password.String)
			rec.Set("passwordConfirm", u.Password.String)
		}
	} else if rec.Id == "" {
		placeholder := fmt.Sprintf("lm-disabled-%d-%d", u.ID, time.Now().UnixNano())
		rec.Set(pbcore.FieldNamePassword, placeholder)
		rec.Set("passwordConfirm", placeholder)
	}

	if err := s.a.pb.Save(rec); err != nil {
		return nil, err
	}

	return rec, nil
}

func (s *PocketBaseAuthService) lookupRecord(u User, lookupUsername string) (*pbcore.Record, error) {
	if u.ID > 0 {
		rec, err := s.FindByUserID(u.ID)
		if err == nil {
			return rec, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	if uname := strings.TrimSpace(lookupUsername); uname != "" {
		rec, err := s.FindByUsername(uname)
		if err == nil {
			return rec, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	if uname := strings.TrimSpace(u.Username); uname != "" {
		rec, err := s.FindByUsername(uname)
		if err == nil {
			return rec, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	col, err := s.a.pb.FindCollectionByNameOrId(s.a.authCol)
	if err != nil {
		return nil, err
	}
	return pbcore.NewRecord(col), nil
}

// AuthenticatePassword validates username/password against the PocketBase auth record.
func (s *PocketBaseAuthService) AuthenticatePassword(username, password string) (*pbcore.Record, error) {
	rec, err := s.FindByUsername(username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if rec == nil || !rec.ValidatePassword(password) {
		return nil, nil
	}

	return rec, nil
}

// DeleteByUserID removes the mapped auth record for a legacy user ID.
func (s *PocketBaseAuthService) DeleteByUserID(userID int) error {
	rec, err := s.FindByUserID(userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if rec == nil {
		return nil
	}
	return s.a.pb.Delete(rec)
}
