package core

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"github.com/pocketbase/dbx"
	pbcore "github.com/pocketbase/pocketbase/core"
	null "gopkg.in/volatiletech/null.v6"
)

func isRoleNameUniqueErr(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") && strings.Contains(msg, "roles.type, roles.name")
}

func roleFromRecord(rec *pbcore.Record) auth.Role {
	out := auth.Role{}

	if rec == nil {
		return out
	}

	if legacyID := parseIntVal(rec.Get("legacy_id")); legacyID > 0 {
		out.ID = legacyID
	} else if id, err := strconv.Atoi(rec.Id); err == nil {
		out.ID = id
	}

	name := strings.TrimSpace(rec.GetString("name"))
	out.Name = null.NewString(name, name != "")
	out.Type = rec.GetString("type")
	out.Permissions = parsePermissions(rec.Get("permissions"))
	out.ListID = null.NewInt(parseIntVal(rec.Get("list_id")), true)

	parentID := strings.TrimSpace(rec.GetString("parent_id"))
	if parentID != "" {
		if parentInt, err := strconv.Atoi(parentID); err == nil {
			out.ParentID = null.NewInt(parentInt, true)
		}
	}

	return out
}

func parsePermissions(raw any) pq.StringArray {
	if raw == nil {
		return pq.StringArray{}
	}

	switch value := raw.(type) {
	case []string:
		return pq.StringArray(value)
	case []any:
		out := make([]string, 0, len(value))
		for _, entry := range value {
			if text, ok := entry.(string); ok && text != "" {
				out = append(out, text)
			}
		}
		return pq.StringArray(out)
	case string:
		if strings.TrimSpace(value) == "" {
			return pq.StringArray{}
		}

		var out []string
		if err := json.Unmarshal([]byte(value), &out); err == nil {
			return pq.StringArray(out)
		}

		return pq.StringArray{value}
	default:
		return pq.StringArray{}
	}
}

func parseIntVal(raw any) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		out, _ := strconv.Atoi(strings.TrimSpace(value))
		return out
	default:
		return 0
	}
}

func (c *Core) useSQLRoleStore() bool {
	pb := c.db.PocketBase()
	if pb == nil {
		return true
	}

	_, err := pb.FindCollectionByNameOrId("roles")
	return err != nil
}

func (c *Core) getRoleByID(id int) (*pbcore.Record, error) {
	if err := c.ensureRolesCollection(); err != nil {
		return nil, err
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return nil, fmt.Errorf("pocketbase instance is nil")
	}

	return pb.FindFirstRecordByFilter("roles", "legacy_id={:id}", dbx.Params{"id": id})
}

func (c *Core) getRoleChildren(parentID string) ([]*pbcore.Record, error) {
	if err := c.ensureRolesCollection(); err != nil {
		return nil, err
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return nil, fmt.Errorf("pocketbase instance is nil")
	}

	escaped := strings.ReplaceAll(parentID, "'", "''")
	return pb.FindRecordsByFilter("roles", "parent_id='"+escaped+"' && type='list'", "", 0, 0)
}

func (c *Core) ensureRolesCollection() error {
	pb := c.db.PocketBase()
	if pb == nil {
		return fmt.Errorf("pocketbase instance is nil")
	}

	col, err := pb.FindCollectionByNameOrId("roles")
	if err == nil {
		changed := false

		if col.Fields.GetByName("legacy_id") == nil {
			col.Fields.Add(&pbcore.NumberField{Name: "legacy_id", Required: true, OnlyInt: true})
			changed = true
		}
		if col.Fields.GetByName("type") == nil {
			col.Fields.Add(&pbcore.TextField{Name: "type", Required: true})
			changed = true
		}
		if col.Fields.GetByName("name") == nil {
			col.Fields.Add(&pbcore.TextField{Name: "name"})
			changed = true
		}
		if col.Fields.GetByName("permissions") == nil {
			col.Fields.Add(&pbcore.JSONField{Name: "permissions"})
			changed = true
		}
		if col.Fields.GetByName("parent_id") == nil {
			col.Fields.Add(&pbcore.TextField{Name: "parent_id"})
			changed = true
		}
		if col.Fields.GetByName("list_id") == nil {
			col.Fields.Add(&pbcore.NumberField{Name: "list_id", OnlyInt: true})
			changed = true
		}
		if col.Fields.GetByName("created") == nil {
			col.Fields.Add(&pbcore.AutodateField{Name: "created", OnCreate: true})
			changed = true
		}
		if col.Fields.GetByName("updated") == nil {
			col.Fields.Add(&pbcore.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
			changed = true
		}

		if changed {
			col.AddIndex("idx_roles_legacy_id", true, "legacy_id", "")
			if err := pb.Save(col); err != nil {
				return err
			}
		}

		return nil
	}

	col = pbcore.NewBaseCollection("roles")
	col.Fields.Add(
		&pbcore.NumberField{Name: "legacy_id", Required: true, OnlyInt: true},
		&pbcore.TextField{Name: "type", Required: true},
		&pbcore.TextField{Name: "name"},
		&pbcore.JSONField{Name: "permissions"},
		&pbcore.TextField{Name: "parent_id"},
		&pbcore.NumberField{Name: "list_id", OnlyInt: true},
		&pbcore.AutodateField{Name: "created", OnCreate: true},
		&pbcore.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	col.AddIndex("idx_roles_legacy_id", true, "legacy_id", "")

	return pb.Save(col)
}

func (c *Core) nextRoleLegacyID() (int, error) {
	pb := c.db.PocketBase()
	if pb == nil {
		return 0, fmt.Errorf("pocketbase instance is nil")
	}

	recs, err := pb.FindRecordsByFilter("roles", "", "-legacy_id", 1, 0)
	if err != nil || len(recs) == 0 {
		return 1, nil
	}

	return parseIntVal(recs[0].Get("legacy_id")) + 1, nil
}

// GetRoles retrieves all roles.
func (c *Core) GetRoles() ([]auth.Role, error) {
	if err := c.ensureRolesCollection(); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "role", "error", err.Error()))
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "role", "error", "pocketbase unavailable"))
	}

	recs, err := pb.FindRecordsByFilter("roles", "type='user'", "created", 0, 0)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "role", "error", err.Error()))
	}

	out := make([]auth.Role, 0, len(recs))
	for _, rec := range recs {
		if strings.TrimSpace(rec.GetString("parent_id")) != "" {
			continue
		}
		out = append(out, roleFromRecord(rec))
	}

	return out, nil
}

// GetRole retrieves a role.
func (c *Core) GetRole(id int) (auth.Role, error) {
	if c.useSQLRoleStore() {
		out := []auth.Role{}
		if err := c.q.GetUserRoles.Select(&out, id); err != nil {
			return auth.Role{}, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "role", "error", pqErrMsg(err)))
		}

		if len(out) == 0 {
			return auth.Role{}, echo.NewHTTPError(http.StatusNotFound,
				c.i18n.Ts("globals.messages.notFound", "name", "role"))
		}

		return out[0], nil
	}

	rec, err := c.getRoleByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return auth.Role{}, echo.NewHTTPError(http.StatusNotFound,
				c.i18n.Ts("globals.messages.notFound", "name", "role"))
		}

		return auth.Role{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "role", "error", err.Error()))
	}

	return roleFromRecord(rec), nil
}

// GetListRoles retrieves all list roles.
func (c *Core) GetListRoles() ([]auth.ListRole, error) {
	if err := c.ensureRolesCollection(); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "role", "error", err.Error()))
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "role", "error", "pocketbase unavailable"))
	}

	recs, err := pb.FindRecordsByFilter("roles", "type='list'", "created", 0, 0)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "role", "error", err.Error()))
	}

	listNames := map[int]string{}
	rows := []struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}{}
	if err := c.db.Select(&rows, "SELECT id, name FROM lists"); err == nil {
		for _, row := range rows {
			listNames[row.ID] = row.Name
		}
	}

	parents := map[string]*auth.ListRole{}
	for _, rec := range recs {
		parentID := strings.TrimSpace(rec.GetString("parent_id"))
		if parentID != "" {
			continue
		}

		role := roleFromRecord(rec)
		outRole := auth.ListRole{Base: role.Base, Name: role.Name, Lists: []auth.ListPermission{}}
		parents[rec.Id] = &outRole
	}

	for _, rec := range recs {
		parentID := strings.TrimSpace(rec.GetString("parent_id"))
		if parentID == "" {
			continue
		}

		parent, ok := parents[parentID]
		if !ok {
			continue
		}

		listID := parseIntVal(rec.Get("list_id"))
		if listID <= 0 {
			continue
		}

		parent.Lists = append(parent.Lists, auth.ListPermission{
			ID:          listID,
			Name:        listNames[listID],
			Permissions: parsePermissions(rec.Get("permissions")),
		})
	}

	out := make([]auth.ListRole, 0, len(parents))
	for _, role := range parents {
		out = append(out, *role)
	}

	return out, nil
}

// CreateRole creates a new role.
func (c *Core) CreateRole(r auth.Role) (auth.Role, error) {
	if c.useSQLRoleStore() {
		var out auth.Role
		if err := c.q.CreateRole.Get(&out, r.Name, auth.RoleTypeUser, pq.Array(r.Permissions)); err != nil {
			if isRoleNameUniqueErr(err) {
				roles, getErr := c.GetRoles()
				if getErr == nil {
					for _, role := range roles {
						if role.Name.Valid && r.Name.Valid && role.Name.String == r.Name.String {
							return role, nil
						}
					}
				}
			}

			return out, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", pqErrMsg(err)))
		}

		return out, nil
	}

	if err := c.ensureRolesCollection(); err != nil {
		return auth.Role{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", err.Error()))
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return auth.Role{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", "pocketbase unavailable"))
	}

	if r.Name.Valid {
		rec, err := pb.FindFirstRecordByFilter("roles", "type={:type} && name={:name} && parent_id=''", dbx.Params{
			"type": auth.RoleTypeUser,
			"name": r.Name.String,
		})
		if err == nil && rec != nil {
			return roleFromRecord(rec), nil
		}
	}

	col, err := pb.FindCollectionByNameOrId("roles")
	if err != nil {
		return auth.Role{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", err.Error()))
	}

	rec := pbcore.NewRecord(col)
	legacyID, nextErr := c.nextRoleLegacyID()
	if nextErr != nil {
		return auth.Role{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", nextErr.Error()))
	}
	rec.Set("legacy_id", legacyID)
	rec.Set("name", r.Name.String)
	rec.Set("type", auth.RoleTypeUser)
	rec.Set("permissions", []string(r.Permissions))

	if err := pb.Save(rec); err != nil {
		return auth.Role{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", err.Error()))
	}

	return roleFromRecord(rec), nil
}

// CreateListRole creates a new list role.
func (c *Core) CreateListRole(r auth.ListRole) (auth.ListRole, error) {
	if c.useSQLRoleStore() {
		var out auth.ListRole
		if err := c.q.CreateRole.Get(&out, r.Name, auth.RoleTypeList, pq.Array([]string{})); err != nil {
			if isRoleNameUniqueErr(err) {
				roles, getErr := c.GetListRoles()
				if getErr == nil {
					for _, role := range roles {
						if role.Name.Valid && r.Name.Valid && role.Name.String == r.Name.String {
							return role, nil
						}
					}
				}
			}

			return out, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", pqErrMsg(err)))
		}

		if err := c.UpsertListPermissions(out.ID, r.Lists); err != nil {
			return out, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", pqErrMsg(err)))
		}

		return out, nil
	}

	if err := c.ensureRolesCollection(); err != nil {
		return auth.ListRole{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", err.Error()))
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return auth.ListRole{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", "pocketbase unavailable"))
	}

	var rec *pbcore.Record
	if r.Name.Valid {
		existing, err := pb.FindFirstRecordByFilter("roles", "type={:type} && name={:name} && parent_id=''", dbx.Params{
			"type": auth.RoleTypeList,
			"name": r.Name.String,
		})
		if err == nil && existing != nil {
			rec = existing
		}
	}

	if rec == nil {
		col, err := pb.FindCollectionByNameOrId("roles")
		if err != nil {
			return auth.ListRole{}, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", err.Error()))
		}

		rec = pbcore.NewRecord(col)
		legacyID, nextErr := c.nextRoleLegacyID()
		if nextErr != nil {
			return auth.ListRole{}, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", nextErr.Error()))
		}
		rec.Set("legacy_id", legacyID)
		rec.Set("name", r.Name.String)
		rec.Set("type", auth.RoleTypeList)
		rec.Set("permissions", []string{})

		if err := pb.Save(rec); err != nil {
			return auth.ListRole{}, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", err.Error()))
		}
	}

	id := parseIntVal(rec.Get("legacy_id"))
	out := auth.ListRole{Base: auth.Base{ID: id}, Name: null.NewString(rec.GetString("name"), rec.GetString("name") != "")}

	if err := c.UpsertListPermissions(out.ID, r.Lists); err != nil {
		return out, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", err.Error()))
	}

	return out, nil
}

// UpsertListPermissions upserts permission for a role.
func (c *Core) UpsertListPermissions(roleID int, lp []auth.ListPermission) error {
	pb := c.db.PocketBase()
	if pb == nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", "pocketbase unavailable"))
	}

	parent, err := c.getRoleByID(roleID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{users.role}"))
	}

	children, err := c.getRoleChildren(parent.Id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", err.Error()))
	}

	byListID := map[int]*pbcore.Record{}
	for _, child := range children {
		listID := parseIntVal(child.Get("list_id"))
		if listID > 0 {
			byListID[listID] = child
		}
	}

	keep := map[int]struct{}{}
	for _, permission := range lp {
		if permission.ID <= 0 || len(permission.Permissions) == 0 {
			continue
		}

		keep[permission.ID] = struct{}{}
		rec := byListID[permission.ID]
		if rec == nil {
			col, err := pb.FindCollectionByNameOrId("roles")
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError,
					c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", err.Error()))
			}
			rec = pbcore.NewRecord(col)
			rec.Set("type", auth.RoleTypeList)
			rec.Set("parent_id", parent.Id)
			rec.Set("list_id", permission.ID)
		}

		rec.Set("permissions", []string(permission.Permissions))
		if err := pb.Save(rec); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", err.Error()))
		}
	}

	for listID, child := range byListID {
		if _, ok := keep[listID]; ok {
			continue
		}
		if err := pb.Delete(child); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorDeleting", "name", "{users.role}", "error", err.Error()))
		}
	}

	return nil
}

// DeleteListPermission deletes a list permission entry from a role.
func (c *Core) DeleteListPermission(roleID, listID int) error {
	pb := c.db.PocketBase()
	if pb == nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{users.role}", "error", "pocketbase unavailable"))
	}

	parent, err := c.getRoleByID(roleID)
	if err != nil {
		return nil
	}

	children, err := c.getRoleChildren(parent.Id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{users.role}", "error", err.Error()))
	}

	for _, child := range children {
		if parseIntVal(child.Get("list_id")) != listID {
			continue
		}

		if err := pb.Delete(child); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "foreign key") {
				return echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("users.cantDeleteRole"))
			}

			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorDeleting", "name", "{users.role}", "error", err.Error()))
		}
	}

	return nil
}

// UpdateUserRole updates a given role.
func (c *Core) UpdateUserRole(id int, r auth.Role) (auth.Role, error) {
	rec, err := c.getRoleByID(id)
	if err != nil {
		return auth.Role{}, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.notFound", "name", "{users.userRole}"))
	}

	rec.Set("name", r.Name.String)
	rec.Set("permissions", []string(r.Permissions))

	if err := c.db.PocketBase().Save(rec); err != nil {
		return auth.Role{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{users.userRole}", "error", err.Error()))
	}

	out := roleFromRecord(rec)

	if out.ID == 0 {
		return out, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.notFound", "name", "{users.userRole}"))
	}

	return out, nil
}

// UpdateListRole updates a given role.
func (c *Core) UpdateListRole(id int, r auth.ListRole) (auth.ListRole, error) {
	rec, err := c.getRoleByID(id)
	if err != nil {
		return auth.ListRole{}, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.notFound", "name", "{users.listRole}"))
	}

	rec.Set("name", r.Name.String)
	if err := c.db.PocketBase().Save(rec); err != nil {
		return auth.ListRole{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{users.listRole}", "error", err.Error()))
	}

	idInt, _ := strconv.Atoi(rec.Id)
	out := auth.ListRole{Base: auth.Base{ID: idInt}, Name: null.NewString(rec.GetString("name"), rec.GetString("name") != "")}

	if out.ID == 0 {
		return out, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.notFound", "name", "{users.listRole}"))
	}

	if err := c.UpsertListPermissions(out.ID, r.Lists); err != nil {
		return out, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{users.listRole}", "error", pqErrMsg(err)))
	}

	return out, nil
}

// DeleteRole deletes a given role.
func (c *Core) DeleteRole(id int) error {
	rec, err := c.getRoleByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{users.role}", "error", err.Error()))
	}

	if err := c.db.PocketBase().Delete(rec); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "foreign key") {
			return echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("users.cantDeleteRole"))
		}
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{users.role}", "error", err.Error()))
	}

	return nil
}
