This is an MCP server for Spool.
## Building against a local Spool

`go.mod` depends on Spool from GitHub. To build against a local checkout
instead, create a Go workspace (`go.work` is gitignored, so it stays on your
machine):

```
go work init . ../spool
```

Pass the path to wherever your Spool checkout lives. Delete `go.work`, or set
`GOWORK=off`, to build against the GitHub version again.
