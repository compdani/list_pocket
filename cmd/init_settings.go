package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/compdani/list_pocket/internal/config"
	"github.com/compdani/list_pocket/models"
	"github.com/jmoiron/sqlx/types"
	"github.com/pocketbase/pocketbase"
	pbcore "github.com/pocketbase/pocketbase/core"
)

func initSettings(ko *config.Conf) {
	var s types.JSONText

	if pb != nil {
		if out, ok, err := getPBSettings(pb); err != nil {
			lo.Fatalf("error reading settings from PocketBase: %v", err)
		} else if ok {
			if isLegacyPBSettingsBlob(out) {
				b, err := makeDefaultPBSettings(ko)
				if err != nil {
					lo.Fatalf("error marshaling repaired settings: %v", err)
				}
				if err := setPBSettings(pb, b); err != nil {
					lo.Fatalf("error repairing PocketBase settings: %v", err)
				}
				s = b
			} else {
				s = out
			}
		} else {
			// First run: persist app settings defaults into PocketBase settings.
			b, err := makeDefaultPBSettings(ko)
			if err != nil {
				lo.Fatalf("error marshaling default settings: %v", err)
			}
			if err := setPBSettings(pb, b); err != nil {
				lo.Fatalf("error seeding PocketBase settings: %v", err)
			}
			s = b
		}
	} else {
		lo.Fatalf("pocketbase is not initialized")
	}

	// Setting keys are dot separated, eg: app.favicon_url. Unflatten them into
	// nested maps {app: {favicon_url}}.
	var out map[string]any
	if err := json.Unmarshal(s, &out); err != nil {
		lo.Fatalf("error unmarshalling settings from DB: %v", err)
	}
	if err := ko.LoadMap(out); err != nil {
		lo.Fatalf("error parsing settings from DB: %v", err)
	}

	if strings.TrimSpace(ko.String("app.lang")) == "" {
		if err := ko.Set("app.lang", "en"); err != nil {
			lo.Fatalf("error setting default app.lang: %v", err)
		}
	}
	if strings.TrimSpace(ko.String("upload.provider")) == "" {
		if err := ko.Set("upload.provider", "filesystem"); err != nil {
			lo.Fatalf("error setting default upload.provider: %v", err)
		}
	}
	if strings.TrimSpace(ko.String("upload.filesystem.upload_uri")) == "" {
		if err := ko.Set("upload.filesystem.upload_uri", "/uploads"); err != nil {
			lo.Fatalf("error setting default upload.filesystem.upload_uri: %v", err)
		}
	}
	if strings.TrimSpace(ko.String("upload.filesystem.upload_path")) == "" {
		if err := ko.Set("upload.filesystem.upload_path", "uploads"); err != nil {
			lo.Fatalf("error setting default upload.filesystem.upload_path: %v", err)
		}
	}
}

func isLegacyPBSettingsBlob(raw []byte) bool {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}

	_, hasSiteName := m["app.site_name"]
	_, hasSMTP := m["smtp"]
	_, hasAppAddress := m["app.address"]
	_, hasDBHost := m["db.host"]

	return !hasSiteName && !hasSMTP && (hasAppAddress || hasDBHost)
}

func makeDefaultPBSettings(ko *config.Conf) ([]byte, error) {
	s := models.Settings{
		AppLang:                    "en",
		UploadProvider:             "filesystem",
		UploadFilesystemUploadURI:  "/uploads",
		UploadFilesystemUploadPath: "uploads",
		UploadExtensions:           []string{},
		AppNotifyEmails:            []string{},
		PrivacyExportable:          []string{},
		DomainBlocklist:            []string{},
		DomainAllowlist:            []string{},
		SecurityCORSOrigins:        []string{},
		SMTP: []models.SMTPSettings{
			{
				Enabled:          true,
				Port:             25,
				AuthProtocol:     "login",
				EmailHeaders:     []map[string]string{},
				FromAddresses:    []string{},
				DefaultFromEmail: "",
				MaxConns:         10,
				MaxMsgRetries:    2,
				IdleTimeout:      "15s",
				WaitTimeout:      "5s",
				TLSType:          "none",
			},
		},
		Messengers: []struct {
			UUID          string `json:"uuid"`
			Enabled       bool   `json:"enabled"`
			Name          string `json:"name"`
			RootURL       string `json:"root_url"`
			Username      string `json:"username"`
			Password      string `json:"password,omitempty"`
			MaxConns      int    `json:"max_conns"`
			Timeout       string `json:"timeout"`
			MaxMsgRetries int    `json:"max_msg_retries"`
		}{},
		BounceActions: map[string]struct {
			Count  int    `json:"count"`
			Action string `json:"action"`
		}{
			"soft":      {Count: 1, Action: "none"},
			"hard":      {Count: 1, Action: "none"},
			"complaint": {Count: 1, Action: "none"},
		},
		BounceBoxes: []struct {
			UUID          string `json:"uuid"`
			Enabled       bool   `json:"enabled"`
			Type          string `json:"type"`
			Host          string `json:"host"`
			Port          int    `json:"port"`
			AuthProtocol  string `json:"auth_protocol"`
			ReturnPath    string `json:"return_path"`
			Username      string `json:"username"`
			Password      string `json:"password,omitempty"`
			TLSEnabled    bool   `json:"tls_enabled"`
			TLSSkipVerify bool   `json:"tls_skip_verify"`
			ScanInterval  string `json:"scan_interval"`
		}{
			{
				Type:         "pop",
				Port:         110,
				AuthProtocol: "userpass",
				ScanInterval: "15m",
			},
		},
	}

	if v := strings.TrimSpace(ko.String("app.lang")); v != "" {
		s.AppLang = v
	}
	if v := strings.TrimSpace(ko.String("upload.provider")); v != "" {
		s.UploadProvider = v
	}
	if v := strings.TrimSpace(ko.String("upload.filesystem.upload_uri")); v != "" {
		s.UploadFilesystemUploadURI = v
	}
	if v := strings.TrimSpace(ko.String("upload.filesystem.upload_path")); v != "" {
		s.UploadFilesystemUploadPath = v
	}

	return json.Marshal(s)
}

func initPocketBase() *pocketbase.PocketBase {
	// PocketBase is created here but bootstrapped later via pb.Start()/Execute(),
	// matching the normal embedded-PocketBase lifecycle.
	return pocketbase.NewWithConfig(pocketbase.Config{
		HideStartBanner: true,
		DefaultDataDir:  "pb_data",
	})
}

func getPBSettings(pb *pocketbase.PocketBase) (types.JSONText, bool, error) {
	var row struct {
		Value []byte `db:"value"`
	}

	queries := []string{
		"SELECT value FROM listpocket_settings WHERE type='app' LIMIT 1",
		"SELECT value FROM listpocket_settings LIMIT 1",
		"SELECT value FROM listmonk_settings LIMIT 1",
	}
	for _, q := range queries {
		err := pb.DB().NewQuery(q).One(&row)
		if err == nil {
			return row.Value, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			continue
		}
	}

	return nil, false, nil
}

func setPBSettings(pb *pocketbase.PocketBase, value []byte) error {
	var (
		collection *pbcore.Collection
		err        error
	)

	for _, name := range []string{"listpocket_settings", "listmonk_settings"} {
		collection, err = pb.FindCollectionByNameOrId(name)
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}

	// Check if an app settings record already exists.
	var existingRecord *pbcore.Record
	records, err := pb.FindRecordsByFilter(collection, "type='app'", "", 1, 0)
	if err == nil && len(records) > 0 {
		existingRecord = records[0]
	}
	if existingRecord == nil {
		// Backward compatibility with older databases that had no `type` field set.
		records, err = pb.FindRecordsByFilter(collection, "", "", 1, 0)
		if err == nil && len(records) > 0 {
			existingRecord = records[0]
		}
	}

	if existingRecord != nil {
		// Update existing record
		existingRecord.Set("value", string(value))
		if collection.Fields.GetByName("type") != nil {
			existingRecord.Set("type", "app")
		}
		return pb.Save(existingRecord)
	} else {
		// Create new record
		record := pbcore.NewRecord(collection)
		record.Set("value", string(value))
		if collection.Fields.GetByName("type") != nil {
			record.Set("type", "app")
		}
		return pb.Save(record)
	}
}

func patchPBSettings(pb *pocketbase.PocketBase, key string, value json.RawMessage) error {
	raw, ok, err := getPBSettings(pb)
	if err != nil {
		return err
	}
	if !ok {
		raw = []byte("{}")
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}

	var out any
	if err := json.Unmarshal(value, &out); err != nil {
		return err
	}
	m[key] = out

	b, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return setPBSettings(pb, b)
}
