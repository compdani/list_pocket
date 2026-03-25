If you spot a security vulnerability in List Pocket, please report it via GitHub [security advisories](https://github.com/compdani/list_pocket/security/advisories) on this repository.

## Notes

### Subscriber SQL queries

The subscribers UI (and APIs) support issuing of arbitrary SQL expressions via a `query` parameter. While List Pocket ensures that the queries are executed as readonly and has basic checks for target tables to prevent accidental side-effects, it is not really possible to prevent arbitrary Turing-complete SQL expressions from calling various Postgres functions. Postgres itself does not offer an easy way to allow/disallow specific functions.

That's why this feature is behind a special permission `subscribers:sql_query` and its risks are documented in [User roles and permissions](roles-and-permissions.md#user-roles). In a multi-user scenario, it is up to an admin to allow this permission to trusted users.

### Media uploads

In addition to images, List Pocket allows uploading of arbitrary file types, .html, .js, .svg, .* and does not transform or modify the files. That means, it is possible to have `<script>`s and other arbitrary content inside HTML and SVG files. It is not possible for the app to have special checks or transformations for various file types, and many environments legitimately want SVGs and other filetypes to be uploaded.

### Campaign HTML

List Pocket is a full-fledged HTML content management system (similar to WordPress). It allows campaign messages to have arbitrary HTML, including `<script>`s, which is a legitimate use case in many environments. While campaign previews within the admin are iframe-sandboxed, when a campaign is published as a webpage (archive view), it will obviously execute whatever `<script>` it has. This is expected of a content management and publishing system. In a multi-user scenario, it is up to the admin to give appropriate permissions to trusted users if they deem this undesirable.

### Templates

List Pocket is a full-fledged HTML content management system (similar to WordPress) which provides the full power of the Go templating language in addition to bundling [Sprig functions](https://masterminds.github.io/sprig/). This allows executing Turing-complete code within templates where it is possible to write code that does loop-within-loop or memory-allocation code that runs within loops that could cause a DoS. There is no way to prevent this. In a multi-user scenario, it is up to the admin to give appropriate permissions to trusted users if they deem this undesirable.
