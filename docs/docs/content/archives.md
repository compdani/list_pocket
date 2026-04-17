# Archives

A global public archive is maintained on the public web interface. It can be
enabled under Settings -> Settings -> General -> Enable public mailing list
archive.

To make a campaign available in the public archive (provided it has been
enabled in the settings as described above), enable the option
'Publish to public archive' under Campaigns -> Create new -> Archive.

When using template variables that depend on subscriber data (such as any
template variable referencing `.Subscriber`), such data must be supplied
as 'Campaign metadata', which is a JSON object that will be used in place
of `.Subscriber` when rendering the archive template and content.

When individual subscriber tracking is enabled, TrackLink requires metadata for
an existing subscriber (including the PocketBase **record id** in the `id` field).
Any clicks on a TrackLink from the archived campaign will be counted towards that subscriber.

As an example:

```json
{
  "id": "pbc_subscriber_record_id_here",
  "email": "example@example.com",
  "name": "Reader",
  "attribs": {}
}
```

![Archive campaign](images/archived-campaign-metadata.png)

