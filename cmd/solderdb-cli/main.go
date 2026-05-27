// solderdb-cli is a tiny terminal client for SolderDB. It talks to the local
// REST API via the solderdb-go SDK and persists the auth token under
// $HOME/.solderdb/token so commands chain without re-logging in.
//
// Usage:
//
//	solderdb login <email> <password>
//	solderdb register <email> <password>
//	solderdb whoami
//	solderdb stats
//	solderdb logs [limit]
//
//	solderdb coll ls
//	solderdb coll create <name> <field:type[:required]> ...
//	solderdb coll rm <name>
//
//	solderdb rec ls <coll>
//	solderdb rec get <coll> <id>
//	solderdb rec add <coll> <key=value>... [key=@file.json]
//	solderdb rec rm <coll> <id>
//
//	solderdb file ls
//	solderdb file up <path>
//	solderdb file rm <id>
//
// Flags:
//
//	--url=http://localhost:8787   override the server URL
//	--token=...                   one-shot token (skips disk store)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/N9601/SolderDB/sdk/solderdb-go"
)

const tokenFileName = "token"

type cliCtx struct {
	client *solderdb.Client
	flags  map[string]string
}

func main() {
	args, flags := parseArgs(os.Args[1:])
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage()
		return
	}

	url := flags["url"]
	if url == "" {
		url = "http://localhost:8787"
	}
	c := solderdb.New(url)

	token := flags["token"]
	if token == "" {
		token = loadToken()
	}
	if token != "" {
		c.SetToken(token)
	}

	ctx := &cliCtx{client: c, flags: flags}

	switch args[0] {
	case "login":
		ctx.cmdLogin(args[1:])
	case "register":
		ctx.cmdRegister(args[1:])
	case "logout":
		ctx.cmdLogout()
	case "whoami":
		ctx.cmdWhoami()
	case "stats":
		ctx.cmdStats()
	case "logs":
		ctx.cmdLogs(args[1:])

	case "coll", "collections":
		ctx.cmdColl(args[1:])
	case "rec", "records":
		ctx.cmdRec(args[1:])
	case "file", "files":
		ctx.cmdFile(args[1:])

	default:
		fmt.Fprintf(os.Stderr, "solderdb: unknown command %q\n", args[0])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`solderdb — local REST client for SolderDB

Commands:
  login <email> <password>          authenticate and save token
  register <email> <password>       create account (first = admin)
  logout                            forget the saved token
  whoami                            print current user
  stats                             engine telemetry (admin only)
  logs [limit]                      tail recent API activity (admin only)

  coll ls                           list collections
  coll create <name> <field:type[:required]>...
  coll rm <name>                    delete collection + records

  rec ls <coll>                     list records
  rec get <coll> <id>               fetch one record
  rec add <coll> k=v k=v ...        insert record (use k=@file.json for JSON)
  rec rm <coll> <id>                delete one record

  file ls                           list files
  file up <path>                    upload a file
  file rm <id>                      delete one file

Flags:
  --url=http://localhost:8787       server URL override
  --token=...                       one-shot bearer token`)
}

// ---------------- Commands ----------------

func (x *cliCtx) cmdLogin(args []string) {
	if len(args) != 2 {
		fail("login <email> <password>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sess, err := x.client.Auth.Login(ctx, args[0], args[1])
	check(err)
	saveToken(sess.Token)
	fmt.Printf("logged in as %s (%s)\n", sess.User.Email, sess.User.Role)
}

func (x *cliCtx) cmdRegister(args []string) {
	if len(args) != 2 {
		fail("register <email> <password>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sess, err := x.client.Auth.Register(ctx, args[0], args[1])
	check(err)
	saveToken(sess.Token)
	fmt.Printf("registered %s (%s)\n", sess.User.Email, sess.User.Role)
}

func (x *cliCtx) cmdLogout() {
	saveToken("")
	x.client.Auth.Logout()
	fmt.Println("logged out")
}

func (x *cliCtx) cmdWhoami() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	u, err := x.client.Auth.Me(ctx)
	check(err)
	fmt.Printf("%s\t%s\t%s\n", u.Email, u.Role, u.ID)
}

func (x *cliCtx) cmdStats() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := x.client.Admin.Stats(ctx)
	check(err)
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%-16s %v\n", k, s[k])
	}
}

func (x *cliCtx) cmdLogs(args []string) {
	limit := "50"
	if len(args) > 0 {
		limit = args[0]
	}
	body, err := x.rawGET("/api/logs?limit=" + limit)
	check(err)
	var entries []struct {
		Timestamp string `json:"timestamp"`
		Method    string `json:"method"`
		Path      string `json:"path"`
		Status    int    `json:"status"`
		Duration  int64  `json:"durationMs"`
		User      string `json:"user"`
	}
	check(json.Unmarshal(body, &entries))
	for _, e := range entries {
		t := e.Timestamp
		if parsed, err := time.Parse(time.RFC3339Nano, e.Timestamp); err == nil {
			t = parsed.Local().Format("15:04:05.000")
		}
		user := e.User
		if user == "" {
			user = "-"
		}
		fmt.Printf("%-12s  %-6s  %-3d  %-5dms  %-18s  %s\n", t, e.Method, e.Status, e.Duration, user, e.Path)
	}
}

func (x *cliCtx) cmdColl(args []string) {
	if len(args) == 0 {
		fail("coll ls | create | rm")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	switch args[0] {
	case "ls":
		list, err := x.client.Admin.ListCollections(ctx)
		check(err)
		for _, c := range list {
			fmt.Printf("%-24s %d fields\n", c.Name, len(c.Fields))
		}
	case "create":
		if len(args) < 3 {
			fail("coll create <name> <field:type[:required]>...")
		}
		fields := make([]solderdb.Field, 0, len(args)-2)
		for _, raw := range args[2:] {
			parts := strings.Split(raw, ":")
			if len(parts) < 2 {
				fail("field must be name:type[:required]")
			}
			f := solderdb.Field{Name: parts[0], Type: parts[1]}
			if len(parts) >= 3 && (parts[2] == "required" || parts[2] == "req") {
				f.Required = true
			}
			fields = append(fields, f)
		}
		m, err := x.client.Admin.CreateCollection(ctx, solderdb.CollectionMeta{Name: args[1], Fields: fields})
		check(err)
		fmt.Printf("created %s with %d fields\n", m.Name, len(m.Fields))
	case "rm":
		if len(args) != 2 {
			fail("coll rm <name>")
		}
		check(x.client.Admin.DeleteCollection(ctx, args[1]))
		fmt.Println("deleted", args[1])
	default:
		fail("coll: unknown subcommand " + args[0])
	}
}

func (x *cliCtx) cmdRec(args []string) {
	if len(args) < 2 {
		fail("rec ls|get|add|rm <coll> ...")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	coll := x.client.Collection(args[1])
	switch args[0] {
	case "ls":
		res, err := coll.List(ctx, solderdb.ListOptions{Limit: 50})
		check(err)
		for _, r := range res.Records {
			fmt.Printf("%s  %s\n", r.ID, summary(r.Data))
		}
	case "get":
		if len(args) != 3 {
			fail("rec get <coll> <id>")
		}
		d, err := coll.Get(ctx, args[2])
		check(err)
		b, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println(string(b))
	case "add":
		data, err := parseKVPairs(args[2:])
		check(err)
		d, err := coll.Create(ctx, data)
		check(err)
		fmt.Println(d.ID)
	case "rm":
		if len(args) != 3 {
			fail("rec rm <coll> <id>")
		}
		check(coll.Delete(ctx, args[2]))
		fmt.Println("deleted", args[2])
	default:
		fail("rec: unknown subcommand " + args[0])
	}
}

func (x *cliCtx) cmdFile(args []string) {
	if len(args) == 0 {
		fail("file ls | up <path> | rm <id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	switch args[0] {
	case "ls":
		list, _, err := x.client.Files.List(ctx, solderdb.ListOptions{Limit: 50})
		check(err)
		for _, f := range list {
			fmt.Printf("%s  %-10d  %-24s  %s\n", f.ID, f.Size, f.MimeType, f.Name)
		}
	case "up":
		if len(args) != 2 {
			fail("file up <path>")
		}
		path := args[1]
		f, err := os.Open(path)
		check(err)
		defer func() { _ = f.Close() }()
		meta, err := x.client.Files.Upload(ctx, filepath.Base(path), f, "")
		check(err)
		fmt.Printf("%s  %d bytes  %s\n", meta.ID, meta.Size, meta.SHA256)
	case "rm":
		if len(args) != 2 {
			fail("file rm <id>")
		}
		check(x.client.Files.Delete(ctx, args[1]))
		fmt.Println("deleted", args[1])
	default:
		fail("file: unknown subcommand " + args[0])
	}
}

// ---------------- Helpers ----------------

func (x *cliCtx) rawGET(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, x.client.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if tok := x.client.Token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func parseKVPairs(args []string) (map[string]any, error) {
	out := map[string]any{}
	for _, a := range args {
		eq := strings.IndexByte(a, '=')
		if eq < 0 {
			return nil, fmt.Errorf("expected key=value, got %q", a)
		}
		k, v := a[:eq], a[eq+1:]
		// k=@file.json reads JSON file
		if strings.HasPrefix(v, "@") {
			bytes, err := os.ReadFile(v[1:])
			if err != nil {
				return nil, err
			}
			var parsed any
			if err := json.Unmarshal(bytes, &parsed); err != nil {
				return nil, fmt.Errorf("%s: %w", v, err)
			}
			out[k] = parsed
			continue
		}
		// try JSON literal first, then fall back to string
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			out[k] = parsed
		} else {
			out[k] = v
		}
	}
	return out, nil
}

func summary(data map[string]any) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		s := fmt.Sprintf("%v", data[k])
		if len(s) > 40 {
			s = s[:40] + "…"
		}
		parts = append(parts, k+"="+s)
	}
	return strings.Join(parts, " ")
}

func parseArgs(in []string) (positional []string, flags map[string]string) {
	flags = map[string]string{}
	for _, a := range in {
		if strings.HasPrefix(a, "--") {
			body := a[2:]
			eq := strings.IndexByte(body, '=')
			if eq >= 0 {
				flags[body[:eq]] = body[eq+1:]
			} else {
				flags[body] = "true"
			}
			continue
		}
		positional = append(positional, a)
	}
	return positional, flags
}

func tokenPath() string {
	dir, err := os.UserHomeDir()
	if err != nil || dir == "" {
		dir = "."
	}
	return filepath.Join(dir, ".solderdb", tokenFileName)
}

func loadToken() string {
	b, err := os.ReadFile(tokenPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func saveToken(t string) {
	p := tokenPath()
	if t == "" {
		_ = os.Remove(p)
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(p, []byte(t), 0o600)
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "solderdb:", msg)
	os.Exit(2)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
