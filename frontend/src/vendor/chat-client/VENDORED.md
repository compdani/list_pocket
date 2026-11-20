# Vendored chat client

Source: `github.com/compdani/pocketbase-mcp v0.1.6`, `frontend/client/` (the client
package inside the module; its own `package.json` version is 0.1.2).
Do not edit: re-copy to update.
Re-copy from the Go module cache:
`rsync -a --exclude test "$(go env GOMODCACHE)/github.com/compdani/pocketbase-mcp@v0.1.6/frontend/client/" frontend/src/vendor/chat-client/ && chmod u+w frontend/src/vendor/chat-client/*`
