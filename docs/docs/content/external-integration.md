# Integrating with external systems

In many environments, a mailing list manager's subscriber database is not run independently but as a part of an existing customer database or a CRM. There are multiple ways of keeping listmonk in sync with external systems.

## Using APIs

The [subscriber APIs](apis/subscribers.md) offers several APIs to manipulate the subscribers database, like addition, updation, and deletion. For bulk synchronisation, a CSV can be generated (and optionally zipped) and posted to the import API.

## Interacting directly with the DB

listpocket stores subscribers, lists, and subscriptions in PocketBase collections. For advanced integrations in this fork, use the app APIs and PocketBase collection definitions rather than the old SQL schema from upstream listmonk.
