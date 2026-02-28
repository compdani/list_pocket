package models

import (
	"context"
	"database/sql"
	"strings"

	"github.com/knadh/listmonk/internal/pbdb"
	"github.com/lib/pq"
)

// Queries contains all prepared SQL queries.
type Queries struct {
	GetDashboardCharts *pbdb.Query `query:"get-dashboard-charts"`
	GetDashboardCounts *pbdb.Query `query:"get-dashboard-counts"`

	InsertSubscriber                *pbdb.Query `query:"insert-subscriber"`
	UpsertSubscriber                *pbdb.Query `query:"upsert-subscriber"`
	UpsertBlocklistSubscriber       *pbdb.Query `query:"upsert-blocklist-subscriber"`
	GetSubscriber                   *pbdb.Query `query:"get-subscriber"`
	HasSubscriberLists              *pbdb.Query `query:"has-subscriber-list"`
	GetSubscribersByEmails          *pbdb.Query `query:"get-subscribers-by-emails"`
	GetSubscriberLists              *pbdb.Query `query:"get-subscriber-lists"`
	GetSubscriptions                *pbdb.Query `query:"get-subscriptions"`
	GetSubscriberListsLazy          *pbdb.Query `query:"get-subscriber-lists-lazy"`
	UpdateSubscriber                *pbdb.Query `query:"update-subscriber"`
	UpdateSubscriberWithLists       *pbdb.Query `query:"update-subscriber-with-lists"`
	BlocklistSubscribers            *pbdb.Query `query:"blocklist-subscribers"`
	AddSubscribersToLists           *pbdb.Query `query:"add-subscribers-to-lists"`
	DeleteSubscriptions             *pbdb.Query `query:"delete-subscriptions"`
	DeleteUnconfirmedSubscriptions  *pbdb.Query `query:"delete-unconfirmed-subscriptions"`
	ConfirmSubscriptionOptin        *pbdb.Query `query:"confirm-subscription-optin"`
	UnsubscribeSubscribersFromLists *pbdb.Query `query:"unsubscribe-subscribers-from-lists"`
	DeleteSubscribers               *pbdb.Query `query:"delete-subscribers"`
	DeleteBlocklistedSubscribers    *pbdb.Query `query:"delete-blocklisted-subscribers"`
	DeleteOrphanSubscribers         *pbdb.Query `query:"delete-orphan-subscribers"`
	UnsubscribeByCampaign           *pbdb.Query `query:"unsubscribe-by-campaign"`
	ExportSubscriberData            *pbdb.Query `query:"export-subscriber-data"`
	GetSubscriberActivity           *pbdb.Query `query:"get-subscriber-activity"`

	// Non-prepared arbitrary subscriber queries.
	QuerySubscribers                       string      `query:"query-subscribers"`
	QuerySubscribersCount                  string      `query:"query-subscribers-count"`
	QuerySubscribersCountAll               *pbdb.Query `query:"query-subscribers-count-all"`
	QuerySubscribersForExport              string      `query:"query-subscribers-for-export"`
	QuerySubscribersTpl                    string      `query:"query-subscribers-template"`
	DeleteSubscribersByQuery               string      `query:"delete-subscribers-by-query"`
	AddSubscribersToListsByQuery           string      `query:"add-subscribers-to-lists-by-query"`
	BlocklistSubscribersByQuery            string      `query:"blocklist-subscribers-by-query"`
	DeleteSubscriptionsByQuery             string      `query:"delete-subscriptions-by-query"`
	UnsubscribeSubscribersFromListsByQuery string      `query:"unsubscribe-subscribers-from-lists-by-query"`

	CreateList      *pbdb.Query `query:"create-list"`
	QueryLists      string      `query:"query-lists"`
	GetLists        *pbdb.Query `query:"get-lists"`
	GetListsByOptin *pbdb.Query `query:"get-lists-by-optin"`
	GetListTypes    *pbdb.Query `query:"get-list-types"`
	UpdateList      *pbdb.Query `query:"update-list"`
	UpdateListsDate *pbdb.Query `query:"update-lists-date"`
	DeleteLists     *pbdb.Query `query:"delete-lists"`

	CreateCampaign        *pbdb.Query `query:"create-campaign"`
	QueryCampaigns        string      `query:"query-campaigns"`
	GetCampaign           *pbdb.Query `query:"get-campaign"`
	GetCampaignForPreview *pbdb.Query `query:"get-campaign-for-preview"`
	GetCampaignStats      *pbdb.Query `query:"get-campaign-stats"`
	GetCampaignStatus     *pbdb.Query `query:"get-campaign-status"`
	GetArchivedCampaigns  *pbdb.Query `query:"get-archived-campaigns"`
	CampaignHasLists      *pbdb.Query `query:"campaign-has-lists"`

	// These two queries are read as strings and based on settings.individual_tracking=on/off,
	// are interpolated and copied to view and click counts. Same query, different tables.
	GetCampaignAnalyticsCounts string      `query:"get-campaign-analytics-counts"`
	GetCampaignViewCounts      *pbdb.Query `query:"get-campaign-view-counts"`
	GetCampaignClickCounts     *pbdb.Query `query:"get-campaign-click-counts"`
	GetCampaignLinkCounts      *pbdb.Query `query:"get-campaign-link-counts"`
	GetCampaignBounceCounts    *pbdb.Query `query:"get-campaign-bounce-counts"`
	DeleteCampaignViews        *pbdb.Query `query:"delete-campaign-views"`
	DeleteCampaignLinkClicks   *pbdb.Query `query:"delete-campaign-link-clicks"`

	NextCampaigns            *pbdb.Query `query:"next-campaigns"`
	GetRunningCampaign       *pbdb.Query `query:"get-running-campaign"`
	NextCampaignSubscribers  *pbdb.Query `query:"next-campaign-subscribers"`
	GetOneCampaignSubscriber *pbdb.Query `query:"get-one-campaign-subscriber"`
	UpdateCampaign           *pbdb.Query `query:"update-campaign"`
	UpdateCampaignStatus     *pbdb.Query `query:"update-campaign-status"`
	UpdateCampaignCounts     *pbdb.Query `query:"update-campaign-counts"`
	UpdateCampaignArchive    *pbdb.Query `query:"update-campaign-archive"`
	RegisterCampaignView     *pbdb.Query `query:"register-campaign-view"`
	DeleteCampaign           *pbdb.Query `query:"delete-campaign"`
	DeleteCampaigns          *pbdb.Query `query:"delete-campaigns"`

	InsertMedia *pbdb.Query `query:"insert-media"`
	GetMedia    *pbdb.Query `query:"get-media"`
	QueryMedia  *pbdb.Query `query:"query-media"`
	DeleteMedia *pbdb.Query `query:"delete-media"`

	CreateTemplate     *pbdb.Query `query:"create-template"`
	GetTemplates       *pbdb.Query `query:"get-templates"`
	UpdateTemplate     *pbdb.Query `query:"update-template"`
	SetDefaultTemplate *pbdb.Query `query:"set-default-template"`
	DeleteTemplate     *pbdb.Query `query:"delete-template"`

	CreateLink        *pbdb.Query `query:"create-link"`
	GetLinkURL        *pbdb.Query `query:"get-link-url"`
	RegisterLinkClick *pbdb.Query `query:"register-link-click"`

	GetSettings         *pbdb.Query `query:"get-settings"`
	UpdateSettings      *pbdb.Query `query:"update-settings"`
	UpdateSettingsByKey *pbdb.Query `query:"update-settings-by-key"`

	// GetStats *pbdb.Query `query:"get-stats"`
	RecordBounce                *pbdb.Query `query:"record-bounce"`
	QueryBounces                string      `query:"query-bounces"`
	BlocklistBouncedSubscribers *pbdb.Query `query:"blocklist-bounced-subscribers"`
	DeleteBounces               *pbdb.Query `query:"delete-bounces"`
	DeleteBouncesBySubscriber   *pbdb.Query `query:"delete-bounces-by-subscriber"`
	GetDBInfo                   string      `query:"get-db-info"`

	CreateUser        *pbdb.Query `query:"create-user"`
	UpdateUser        *pbdb.Query `query:"update-user"`
	UpdateUserProfile *pbdb.Query `query:"update-user-profile"`
	UpdateUserLogin   *pbdb.Query `query:"update-user-login"`
	SetUserTwoFA      *pbdb.Query `query:"set-user-twofa"`
	DeleteUsers       *pbdb.Query `query:"delete-users"`
	GetUsers          *pbdb.Query `query:"get-users"`
	GetUser           *pbdb.Query `query:"get-user"`
	GetAPITokens      *pbdb.Query `query:"get-api-tokens"`
	LoginUser         *pbdb.Query `query:"login-user"`

	CreateRole            *pbdb.Query `query:"create-role"`
	GetUserRoles          *pbdb.Query `query:"get-user-roles"`
	GetListRoles          *pbdb.Query `query:"get-list-roles"`
	UpdateRole            *pbdb.Query `query:"update-role"`
	DeleteRole            *pbdb.Query `query:"delete-role"`
	UpsertListPermissions *pbdb.Query `query:"upsert-list-permissions"`
	DeleteListPermission  *pbdb.Query `query:"delete-list-permission"`
}

// compileSubscriberQueryTpl takes an arbitrary WHERE expressions
// to filter subscribers from the subscribers table and prepares a query
// out of it using the raw `query-subscribers-template` query template.
// While doing this, a readonly transaction is created and the query is
// dry run on it to ensure that it is indeed readonly.
func (q *Queries) compileSubscriberQueryTpl(searchStr, queryExp string, db *pbdb.DB, subStatus string) (string, error) {
	tx, err := db.BeginTxx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	// There's an arbitrary query condition.
	cond := "TRUE"
	if queryExp != "" {
		cond = queryExp
	}

	// Perform the dry run.
	stmt := strings.ReplaceAll(q.QuerySubscribersTpl, "%query%", cond)
	if _, err := tx.Exec(stmt, true, pq.Int64Array{}, subStatus, searchStr); err != nil {
		return "", err
	}

	return stmt, nil
}

// compileSubscriberQueryTpl takes an arbitrary WHERE expressions and a subscriber
// query template that depends on the filter (eg: delete by query, blocklist by query etc.)
// combines and executes them.
func (q *Queries) ExecSubQueryTpl(searchStr, queryExp, baseQueryTpl string, listIDs []int, db *pbdb.DB, subStatus string, args ...any) error {
	// Perform a dry run.
	filterExp, err := q.compileSubscriberQueryTpl(searchStr, queryExp, db, subStatus)
	if err != nil {
		return err
	}

	if len(listIDs) == 0 {
		listIDs = []int{}
	}

	// Insert the subscriber filter query into the target query.
	stmt := strings.ReplaceAll(baseQueryTpl, "%query%", filterExp)

	// First argument is the boolean indicating if the query is a dry run.
	a := append([]any{false, pq.Array(listIDs), subStatus, searchStr}, args...)

	// Execute the query on the DB.
	if _, err := db.Exec(stmt, a...); err != nil {
		return err
	}
	return nil
}
