# solderdb-go

Go client library for [SolderDB](https://github.com/N9601/SolderDB).

Stdlib-only, no third-party deps.

## Install

```bash
go get github.com/N9601/SolderDB/sdk/solderdb-go
```

## Quick start

```go
package main

import (
    "context"
    "fmt"

    "github.com/N9601/SolderDB/sdk/solderdb-go"
)

func main() {
    ctx := context.Background()
    c := solderdb.New("http://localhost:8787")

    if _, err := c.Auth.Login(ctx, "you@example.com", "supersecret"); err != nil {
        panic(err)
    }

    notes := c.Collection("notes")
    doc, _ := notes.Create(ctx, map[string]any{"title": "hello from go"})
    fmt.Println("created", doc.ID)

    stop, _ := notes.Subscribe(ctx, func(evt solderdb.Event) {
        fmt.Println(evt.Kind, evt.ID)
    })
    defer stop()

    list, _ := notes.List(ctx, solderdb.ListOptions{Limit: 50})
    for _, r := range list.Records {
        fmt.Println(r.ID, r.Data)
    }
}
```

## API

- `c.Auth.Register / Login / Me / Logout`
- `c.Collection(name).List / Get / Create / Update / Delete / Subscribe`
- `c.Admin.ListCollections / CreateCollection / UpdateCollection / DeleteCollection / Stats`
- `c.Files.List / Upload / Delete / URL`

Errors come back as `*solderdb.APIError` with `.Status` and `.Message` fields.
