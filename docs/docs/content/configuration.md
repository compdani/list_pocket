# Configuration

### TOML Configuration file
One or more TOML files can be read by passing `--config config.toml` multiple times. Apart from a few low-level options (such as `[app]` and built-in docs paths), all other settings can be managed from the `Settings` dashboard on the admin UI. **There is no `[db]` block:** persistence is entirely through embedded PocketBase (SQLite in `pb_data/` by default).

To generate a new sample configuration file, run `listpocket --new-config` (or `go run ./cmd -- --new-config` during development).

### Environment variables
Variables in `config.toml` can also be provided as environment variables prefixed by `LISTPOCKET_`, with periods replaced by `__` (double underscore). To start List Pocket using only environment variables, set the variables and pass `--config=""`.

Example:

| **Environment variable**       | Example value  |
| ------------------------------ | -------------- |
| `LISTPOCKET_app__address`      | "0.0.0.0:9000" |
| `LISTPOCKET_app__docs_dir`    | "/var/listpocket/mkdocs-out" |
| `LISTPOCKET_app__swagger_dir` | "/var/listpocket/swagger" |

PocketBase’s own flags (for example data directory) are passed through the PocketBase CLI embedded in the binary; see [PocketBase configuration](https://pocketbase.io/docs/).

### Built-in documentation URLs

If `docs/docs/_out` (MkDocs `site_dir`) and `docs/swagger` exist relative to the process working directory—or you set `app.docs_dir` / `app.swagger_dir`—the HTTP server exposes:

| Path | Content |
| ---- | ------- |
| `/docs/` | MkDocs manual (static HTML) |
| `/openapi.yaml` | OpenAPI 3 spec (`collections.yaml`) |
| `/swagger/` | Swagger UI for the spec |


### Customizing system templates
See [system templates](templating.md#system-templates).


### HTTP routes
When configuring auth proxies and web application firewalls, use this table.

#### Private admin endpoints.

| Methods | Route              | Description             |
| ------- | ------------------ | ----------------------- |
| `*`     | `/api/*`           | Admin APIs              |
| `GET`   | `/admin/*`         | Admin UI and HTML pages |
| `POST`  | `/webhooks/bounce` | Admin bounce webhook    |


#### Public endpoints to expose to the internet.

| Methods     | Route                 | Description                                   |
| ----------- | --------------------- | --------------------------------------------- |
| `GET, POST` | `/subscription/*`     | HTML subscription pages                       |
| `GET, `     | `/link/*`             | Tracked link redirection                      |
| `GET`       | `/campaign/*`         | Pixel tracking image                          |
| `GET`       | `/public/*`           | Static files for HTML subscription pages      |
| `POST`      | `/webhooks/service/*` | Bounce webhook endpoints for AWS and Sendgrid |
| `GET`       | `/uploads/*`          | The file upload path configured in media settings |


## Media uploads

#### Using filesystem

When configuring `docker` volume mounts for using filesystem media uploads, you can follow either of two approaches. [The second option may be necessary if](https://github.com/knadh/listmonk/issues/1169#issuecomment-1674475945) your setup requires you to use `sudo` for docker commands. 

After making any changes you will need to run `sudo docker compose stop ; sudo docker compose up`. 

And under `https://listpocket.mysite.com/admin/settings` you put `/listpocket/uploads` (paths depend on your deployment). 

#### Using volumes

Using `docker volumes`, you can specify the name of volume and destination for the files to be uploaded inside the container.


```yml
app:
    volumes:
      - type: volume
        source: listpocket-uploads
        target: /listpocket/uploads

volumes:
  listpocket-uploads:
```

!!! note

    This volume is managed by `docker` itself, and you can find the host path with `docker volume inspect` on your stack’s volume name.

#### Using bind mounts

```yml
  app:
    volumes:
      - ./path/on/your/host/:/path/inside/container
```
Eg:
```yml
  app:
    volumes:
      - ./data/uploads:/listpocket/uploads
```
The files will be available inside `/data/uploads` directory on the host machine.

To use the default `uploads` folder:
```yml
  app:
    volumes:
      - ./uploads:/listpocket/uploads
```

## Logs

### Docker

https://docs.docker.com/engine/reference/commandline/logs/
```
sudo docker logs -f
sudo docker logs listpocket_app -t
sudo docker logs --help
```
Container info: `sudo docker inspect` on your compose project’s containers.

Docker logs to `/dev/stdout` and `/dev/stderr`. The logs are collected by the docker daemon and stored in your node's host path (by default). The same can be configured (/etc/docker/daemon.json) in your docker daemon settings to setup other logging drivers, logrotate policy and more, which you can read about [here](https://docs.docker.com/config/containers/logging/configure/).

### Binary

List Pocket logs to `stdout`, which is usually not saved to any file. To save logs to a file you can redirect output, for example `./listpocket > listpocket.log`.

Settings → Logs in the admin UI shows recent log lines from memory; they are cleared when the process restarts.

For a systemd unit, redirect `ExecStart` output to a file under `/var/log` or use `journald` — adapt the [upstream listmonk service example](https://github.com/knadh/listmonk/blob/master/listmonk%40.service) for the `listpocket` binary and your paths.


## Time zone

To change the process time zone (logs, etc.) when using Docker Compose, edit `docker-compose.yml`:
```
environment:
    - TZ=Etc/UTC
```
with any Timezone listed [here](https://en.wikipedia.org/wiki/List_of_tz_database_time_zones). Then run `sudo docker-compose stop ; sudo docker-compose up` after making changes.

## SMTP

### Retries
The `Settings -> SMTP -> Retries` denotes the number of times a message that fails at the moment of sending is retried silently using different connections from the SMTP pool. The messages that fail even after retries are the ones that are logged as errors and ignored.

## SMTP ports
Some server hosts block outgoing SMTP ports (25, 465). You may have to contact your host to unblock them before being able to send e-mails. Eg: [Hetzner](https://docs.hetzner.com/cloud/servers/faq/#why-can-i-not-send-any-mails-from-my-server).


## Performance

### Batch size

The batch size parameter is useful when working with very large lists with millions of subscribers for maximising throughput. It is the number of subscribers that are fetched from the database sequentially in a single cycle (~5 seconds) when a campaign is running. Increasing the batch size uses more memory, but reduces the round trip to the database.
