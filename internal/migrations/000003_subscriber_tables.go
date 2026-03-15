package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// Create subscribers collection
		subscribers := core.NewBaseCollection("subscribers")
		subscribers.Fields.Add(
			&core.TextField{
				Name:     "uuid",
				Required: true,
				Pattern:  `^[a-f0-9\-]{36}$`,
			},
			&core.EmailField{
				Name:     "email",
				Required: true,
			},
			&core.TextField{
				Name:     "name",
				Required: false,
			},
			&core.JSONField{
				Name:     "attribs",
				Required: false,
			},
			&core.SelectField{
				Name:     "status",
				Required: true,
				Values:   []string{"enabled", "disabled", "blocklisted"},
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
		subscribers.AddIndex("idx_subs_uuid", true, "uuid", "")
		subscribers.AddIndex("idx_subs_email", true, "email", "")
		subscribers.AddIndex("idx_subs_status", false, "status", "")

		if err := app.Save(subscribers); err != nil {
			return err
		}

		// Create lists collection
		lists := core.NewBaseCollection("lists")
		lists.Fields.Add(
			&core.TextField{
				Name:     "uuid",
				Required: true,
				Pattern:  `^[a-f0-9\-]{36}$`,
			},
			&core.TextField{
				Name:     "name",
				Required: true,
			},
			&core.SelectField{
				Name:     "type",
				Required: true,
				Values:   []string{"public", "private"},
			},
			&core.SelectField{
				Name:     "optin",
				Required: true,
				Values:   []string{"single", "double"},
			},
			&core.JSONField{
				Name:     "tags",
				Required: false,
			},
			&core.TextField{
				Name:     "description",
				Required: false,
			},
			&core.SelectField{
				Name:     "status",
				Required: true,
				Values:   []string{"active", "inactive"},
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
		lists.AddIndex("idx_lists_uuid", true, "uuid", "")
		lists.AddIndex("idx_lists_status", false, "status", "")

		if err := app.Save(lists); err != nil {
			return err
		}

		// Create subscriber_lists junction collection
		subscriberLists := core.NewBaseCollection("subscriber_lists")
		subscriberLists.Fields.Add(
			&core.RelationField{
				Name:          "subscriber_id",
				Required:      true,
				CollectionId:  subscribers.Id,
				CascadeDelete: true,
			},
			&core.RelationField{
				Name:          "list_id",
				Required:      true,
				CollectionId:  lists.Id,
				CascadeDelete: true,
			},
			&core.SelectField{
				Name:     "status",
				Required: true,
				Values:   []string{"unconfirmed", "confirmed", "unsubscribed"},
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
		subscriberLists.AddIndex("idx_sub_lists_unique", true, "subscriber_id, list_id", "")
		subscriberLists.AddIndex("idx_sub_lists_sub_id", false, "subscriber_id", "")
		subscriberLists.AddIndex("idx_sub_lists_list_id", false, "list_id", "")
		subscriberLists.AddIndex("idx_sub_lists_status", false, "status", "")

		return app.Save(subscriberLists)
	}, func(app core.App) error {
		collections := []string{"subscriber_lists", "lists", "subscribers"}
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
