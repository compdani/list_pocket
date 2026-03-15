package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// Get templates collection reference (created in previous migration)
		templates, err := app.FindCollectionByNameOrId("templates")
		if err != nil {
			return err
		}

		// Create campaigns collection
		campaigns := core.NewBaseCollection("campaigns")
		campaigns.Fields.Add(
			&core.TextField{
				Name:     "uuid",
				Required: true,
				Pattern:  `^[a-f0-9\-]{36}$`,
			},
			&core.TextField{
				Name:     "name",
				Required: true,
			},
			&core.TextField{
				Name:     "subject",
				Required: false,
			},
			&core.EmailField{
				Name:     "from_email",
				Required: false,
			},
			&core.TextField{
				Name:     "body",
				Required: false,
			},
			&core.TextField{
				Name:     "body_source",
				Required: false,
			},
			&core.TextField{
				Name:     "altbody",
				Required: false,
			},
			&core.SelectField{
				Name:     "content_type",
				Required: true,
				Values:   []string{"richtext", "html", "markdown", "plain"},
			},
			&core.DateField{
				Name:     "send_at",
				Required: false,
			},
			&core.JSONField{
				Name:     "headers",
				Required: false,
			},
			&core.JSONField{
				Name:     "attribs",
				Required: false,
			},
			&core.SelectField{
				Name:     "status",
				Required: true,
				Values:   []string{"draft", "scheduled", "running", "paused", "finished", "cancelled"},
			},
			&core.JSONField{
				Name:     "tags",
				Required: false,
			},
			&core.SelectField{
				Name:     "type",
				Required: true,
				Values:   []string{"regular", "optin"},
			},
			&core.SelectField{
				Name:     "messenger",
				Required: true,
				Values:   []string{"email"},
			},
			&core.RelationField{
				Name:         "template_id",
				Required:     false,
				CollectionId: templates.Id,
			},
			&core.NumberField{
				Name: "to_send",
			},
			&core.NumberField{
				Name: "sent",
			},
			&core.NumberField{
				Name: "max_subscriber_id",
			},
			&core.NumberField{
				Name: "last_subscriber_id",
			},
			&core.BoolField{
				Name: "archive",
			},
			&core.TextField{
				Name:     "archive_slug",
				Required: false,
			},
			&core.RelationField{
				Name:         "archive_template_id",
				Required:     false,
				CollectionId: templates.Id,
			},
			&core.JSONField{
				Name:     "archive_meta",
				Required: false,
			},
			&core.DateField{
				Name:     "started_at",
				Required: false,
			},
			&core.AutodateField{
				Name:     "created",
				OnCreate: true,
			},
			&core.AutodateField{
				Name:     "updated",
				OnCreate: true,
				OnUpdate: true,
			},
		)
		campaigns.AddIndex("idx_camps_uuid", true, "uuid", "")
		campaigns.AddIndex("idx_camps_archive_slug", true, "archive_slug", "archive_slug IS NOT NULL AND archive_slug != ''")
		campaigns.AddIndex("idx_camps_status", false, "status", "")
		campaigns.AddIndex("idx_camps_name", false, "name", "")

		if err := app.Save(campaigns); err != nil {
			return err
		}

		// Get lists collection reference
		lists, err := app.FindCollectionByNameOrId("lists")
		if err != nil {
			return err
		}

		// Create campaign_lists junction collection
		campaignLists := core.NewBaseCollection("campaign_lists")
		campaignLists.Fields.Add(
			&core.RelationField{
				Name:         "campaign_id",
				Required:     true,
				CollectionId: campaigns.Id,
			},
			&core.RelationField{
				Name:         "list_id",
				Required:     false,
				CollectionId: lists.Id,
			},
			&core.TextField{
				Name:     "list_name",
				Required: false,
			},
			&core.AutodateField{
				Name:     "created",
				OnCreate: true,
			},
			&core.AutodateField{
				Name:     "updated",
				OnCreate: true,
				OnUpdate: true,
			},
		)
		campaignLists.AddIndex("idx_camp_lists_unique", true, "campaign_id, list_id", "")
		campaignLists.AddIndex("idx_camp_lists_camp_id", false, "campaign_id", "")
		campaignLists.AddIndex("idx_camp_lists_list_id", false, "list_id", "")

		if err := app.Save(campaignLists); err != nil {
			return err
		}

		// Create media collection
		media := core.NewBaseCollection("media")
		media.Fields.Add(
			&core.TextField{
				Name:     "uuid",
				Required: true,
				Pattern:  `^[a-f0-9\-]{36}$`,
			},
			&core.TextField{
				Name:     "provider",
				Required: false,
			},
			&core.TextField{
				Name:     "filename",
				Required: true,
			},
			&core.TextField{
				Name:     "content_type",
				Required: false,
			},
			&core.TextField{
				Name:     "thumb",
				Required: false,
			},
			&core.JSONField{
				Name:     "meta",
				Required: false,
			},
			&core.AutodateField{
				Name:     "created",
				OnCreate: true,
			},
			&core.AutodateField{
				Name:     "updated",
				OnCreate: true,
				OnUpdate: true,
			},
		)
		media.AddIndex("idx_media_uuid", true, "uuid", "")
		media.AddIndex("idx_media_filename", false, "provider, filename", "")

		if err := app.Save(media); err != nil {
			return err
		}

		// Create campaign_media junction collection
		campaignMedia := core.NewBaseCollection("campaign_media")
		campaignMedia.Fields.Add(
			&core.RelationField{
				Name:         "campaign_id",
				Required:     true,
				CollectionId: campaigns.Id,
			},
			&core.RelationField{
				Name:         "media_id",
				Required:     false,
				CollectionId: media.Id,
			},
			&core.TextField{
				Name:     "filename",
				Required: false,
			},
			&core.AutodateField{
				Name:     "created",
				OnCreate: true,
			},
			&core.AutodateField{
				Name:     "updated",
				OnCreate: true,
				OnUpdate: true,
			},
		)
		campaignMedia.AddIndex("idx_camp_media_unique", true, "campaign_id, media_id", "")
		campaignMedia.AddIndex("idx_camp_media_camp_id", false, "campaign_id", "")

		if err := app.Save(campaignMedia); err != nil {
			return err
		}

		// Create links collection
		links := core.NewBaseCollection("links")
		links.Fields.Add(
			&core.TextField{
				Name:     "uuid",
				Required: true,
				Pattern:  `^[a-f0-9\-]{36}$`,
			},
			&core.URLField{
				Name:     "url",
				Required: true,
			},
			&core.AutodateField{
				Name:     "created",
				OnCreate: true,
			},
			&core.AutodateField{
				Name:     "updated",
				OnCreate: true,
				OnUpdate: true,
			},
		)
		links.AddIndex("idx_links_uuid", true, "uuid", "")
		links.AddIndex("idx_links_url", true, "url", "")

		return app.Save(links)
	}, func(app core.App) error {
		collections := []string{"links", "campaign_media", "media", "campaign_lists", "campaigns"}
		for _, name := range collections {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			if err := app.Delete(collection); err != nil {
				return err
			}
		}
		return nil
	})
}
