---
title: Realtime
order: 4
---

# Realtime

SolderDB publishes change events over **Server-Sent Events** (SSE). One TCP connection per subscriber, browser-native, stdlib-only on the server (no WebSocket library).

## Connect

```
GET /api/realtime?topic=<topic>&token=<token>
Accept: text/event-stream
```

The server streams events as they happen until the connection drops or the client closes it. Each event is named after its kind (`create`, `update`, `delete`) with a JSON payload in the `data:` line:

```
event: create
data: {"kind":"create","collection":"notes","id":"01H...","timestamp":"...","data":{...}}

event: update
data: {"kind":"update","collection":"notes","id":"01H...","timestamp":"..."}

event: delete
data: {"kind":"delete","collection":"notes","id":"01H...","timestamp":"..."}
```

## Topics

The hub matches topics by **prefix segments**, separated by `:`. A subscriber to `coll:notes` receives events published on `coll:notes:abc` because `notes` is a prefix of `notes:abc`.

| Topic                  | Receives                                                    |
|------------------------|-------------------------------------------------------------|
| `coll`                 | Every collection event from every collection                |
| `coll:notes`           | All events on the `notes` collection                        |
| `coll:notes:01H...`    | Just events for that specific record                        |
| `coll:_files`          | Every file upload and delete (since files ride on a collection) |
| `logs`                 | Every API request (admin-only, used by the Activity view)   |
| `*` or `coll:*`        | Wildcard (matches anything with the prefix)                 |

## Browser (EventSource)

`EventSource` can't set headers, so pass the token in the query string:

```ts
const es = new EventSource(
  `http://localhost:8787/api/realtime?topic=coll:notes&token=${token}`
);

es.addEventListener("create", (e) => {
  const evt = JSON.parse(e.data);
  console.log("new record", evt.id);
});
```

## JS SDK

```ts
const notes = db.collection("notes");

const stop = notes.subscribe(evt => {
  console.log(evt.kind, evt.id, evt.data);
});

// Later:
stop();
```

## Go SDK

```go
stop, _ := notes.Subscribe(ctx, func(evt solderdb.Event) {
    fmt.Println(evt.Kind, evt.ID)
})
defer stop()
```

The Go SDK parses the SSE framing internally. You just get a typed callback.

## Backpressure

Each subscriber has a buffered channel (default 64 events). If your handler can't keep up, **the hub drops events for that subscriber** rather than blocking writers. This is deliberate. A slow consumer cannot stall the database.

If you need durability (no events lost), poll `/api/collections/.../records?after=<lastID>` periodically as a backstop.

## Reconnect

`EventSource` reconnects automatically on transient disconnects. The SolderDB JS SDK leaves this default in place. If you need custom reconnect logic, you can pass your own subscriber.

There's no resume or "from this event ID" semantics in v1. When you reconnect, you start receiving live events again but you don't get a backlog of what you missed. Use the polling backstop above if that matters.
