// Command wamuxctl is the operator CLI for WaMux's Cloud API surface.
//
// It deliberately talks only to the HTTP API: it never opens the database or
// holds a Meta token. That keeps its credentials scoped to an instance token and
// lets the same binary run from a laptop, CI or a sidecar.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type cli struct {
	stdout io.Writer
	stderr io.Writer
	client *http.Client
}

type invocation struct {
	pos   []string
	vals  map[string]string
	flags map[string]bool
}

var boolFlags = map[string]bool{
	"json": true, "verbose": true, "dry-run": true, "yes": true,
	"help": true, "version": true, "insecure": true,
}

func parseArgs(args []string) (invocation, error) {
	inv := invocation{vals: map[string]string{}, flags: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			inv.pos = append(inv.pos, args[i+1:]...)
			break
		}
		if arg == "-h" || arg == "-v" {
			if arg == "-h" {
				inv.flags["help"] = true
			} else if arg == "-v" {
				inv.flags["verbose"] = true
			}
			continue
		}
		if !strings.HasPrefix(arg, "--") {
			inv.pos = append(inv.pos, arg)
			continue
		}
		name := strings.TrimPrefix(arg, "--")
		if name == "" {
			return inv, errors.New("empty flag")
		}
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			value := name[eq+1:]
			name = name[:eq]
			if boolFlags[name] {
				if value != "true" && value != "false" {
					return inv, fmt.Errorf("--%s expects true or false", name)
				}
				inv.flags[name] = value == "true"
			} else {
				inv.vals[name] = value
			}
			continue
		}
		if boolFlags[name] {
			inv.flags[name] = true
			continue
		}
		if i+1 >= len(args) || (strings.HasPrefix(args[i+1], "--") && !strings.Contains(args[i+1], "=")) {
			return inv, fmt.Errorf("--%s requires a value", name)
		}
		i++
		inv.vals[name] = args[i]
	}
	return inv, nil
}

func (c *cli) value(inv invocation, name, env, def string) string {
	if v, ok := inv.vals[name]; ok {
		return v
	}
	if env != "" {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return def
}

func (c *cli) token(inv invocation) string {
	if t := c.value(inv, "instance-token", "WAMUX_INSTANCE_TOKEN", ""); t != "" {
		return t
	}
	return c.value(inv, "api-key", "WAMUX_API_KEY", "")
}

func (c *cli) baseURL(inv invocation) string {
	return strings.TrimRight(c.value(inv, "url", "WAMUX_URL", "http://localhost:8080"), "/")
}

func (c *cli) timeout(inv invocation) time.Duration {
	raw := c.value(inv, "timeout", "WAMUX_TIMEOUT", "30s")
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 30 * time.Second
	}
	return d
}

type response struct {
	Status int
	Header http.Header
	Body   []byte
}

func (c *cli) do(inv invocation, method, path string, body []byte, contentType, auth string) (*response, error) {
	if inv.flags["dry-run"] {
		_, _ = fmt.Fprintf(c.stdout, "DRY RUN %s %s%s\n", method, c.baseURL(inv), path)
		_, _ = fmt.Fprintf(c.stdout, "apikey: %s\n", redact(auth))
		if contentType != "" {
			_, _ = fmt.Fprintf(c.stdout, "Content-Type: %s\n", contentType)
		}
		if len(body) > 0 {
			_, _ = fmt.Fprintf(c.stdout, "body:\n%s\n", redactBody(body))
		}
		return nil, nil
	}
	req, err := http.NewRequest(method, c.baseURL(inv)+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if auth != "" {
		req.Header.Set("apikey", auth)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if inv.flags["verbose"] {
		_, _ = fmt.Fprintf(c.stderr, "%s %s\n", method, req.URL.Redacted())
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	return &response{Status: resp.StatusCode, Header: resp.Header, Body: data}, nil
}

func redact(s string) string {
	if s == "" {
		return "(none)"
	}
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "…" + s[len(s)-4:]
}

func redactBody(body []byte) string {
	var v any
	if json.Unmarshal(body, &v) != nil {
		return string(body)
	}
	walk := func(m map[string]any) {
		for k := range m {
			lk := strings.ToLower(k)
			if strings.Contains(lk, "token") || strings.Contains(lk, "secret") || strings.Contains(lk, "pin") || strings.Contains(lk, "apikey") {
				m[k] = "***"
			}
		}
	}
	var walkAny func(any)
	walkAny = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			walk(x)
			for _, child := range x {
				walkAny(child)
			}
		case []any:
			for _, child := range x {
				walkAny(child)
			}
		}
	}
	walkAny(v)
	out, _ := json.MarshalIndent(v, "", "  ")
	return string(out)
}

func (c *cli) print(inv invocation, r *response) int {
	if r == nil {
		return 0
	}
	if inv.flags["json"] {
		_, _ = fmt.Fprintln(c.stdout, strings.TrimSpace(string(r.Body)))
	} else {
		var v any
		if json.Unmarshal(r.Body, &v) == nil {
			out, _ := json.MarshalIndent(v, "", "  ")
			_, _ = fmt.Fprintln(c.stdout, string(out))
		} else {
			_, _ = fmt.Fprintln(c.stdout, string(r.Body))
		}
	}
	if r.Status >= 200 && r.Status < 300 {
		return 0
	}
	_, _ = fmt.Fprintf(c.stderr, "HTTP %d\n", r.Status)
	return 1
}

func (c *cli) request(inv invocation, method, path string, body []byte, contentType string) int {
	r, err := c.do(inv, method, path, body, contentType, c.token(inv))
	if err != nil {
		_, _ = fmt.Fprintln(c.stderr, err)
		return 1
	}
	return c.print(inv, r)
}

func jsonBody(v any) ([]byte, string) {
	b, _ := json.Marshal(v)
	return b, "application/json"
}

func (c *cli) readBody(inv invocation) ([]byte, error) {
	raw := inv.vals["body"]
	if raw == "" {
		raw = inv.vals["file"]
	}
	if raw == "" {
		return nil, errors.New("--body or --file is required")
	}
	if raw == "-" {
		return io.ReadAll(os.Stdin)
	}
	raw = strings.TrimPrefix(raw, "@")
	if strings.HasPrefix(strings.TrimSpace(raw), "{") || strings.HasPrefix(strings.TrimSpace(raw), "[") {
		return []byte(raw), nil
	}
	return os.ReadFile(raw)
}

func (c *cli) cloud(inv invocation) int {
	if len(inv.pos) < 2 {
		return c.usage()
	}
	sub := inv.pos[1]
	args := inv.pos[2:]
	switch sub {
	case "doctor":
		return c.doctor(inv)
	case "status":
		return c.request(inv, http.MethodGet, "/instance/status", nil, "")
	case "credentials":
		if len(args) == 0 {
			return c.usage()
		}
		switch args[0] {
		case "get":
			return c.request(inv, http.MethodGet, "/cloud/credentials", nil, "")
		case "set":
			body, err := c.readBody(inv)
			if err != nil {
				payload := map[string]any{}
				for flag, key := range map[string]string{
					"app-id": "appId", "waba-id": "wabaId", "phone-number-id": "phoneNumberId",
					"business-id": "businessId", "access-token": "accessToken",
				} {
					if v := inv.vals[flag]; v != "" {
						payload[key] = v
					}
				}
				if len(payload) == 0 {
					_, _ = fmt.Fprintln(c.stderr, "--body/--file or credential flags are required")
					return 2
				}
				body, _ = json.Marshal(payload)
			}
			return c.request(inv, http.MethodPut, "/cloud/credentials", body, "application/json")
		default:
			return c.usage()
		}
	case "verify":
		return c.request(inv, http.MethodPost, "/cloud/verify", []byte("{}"), "application/json")
	case "register":
		pin := c.value(inv, "pin", "", "")
		if pin == "" {
			_, _ = fmt.Fprintln(c.stderr, "--pin is required")
			return 2
		}
		body, ct := jsonBody(map[string]string{"pin": pin})
		return c.request(inv, http.MethodPost, "/cloud/register", body, ct)
	case "send":
		return c.send(inv, args)
	case "templates", "template":
		return c.templates(inv, args)
	case "account-events":
		return c.request(inv, http.MethodGet, "/cloud/account-events"+query(inv, "field", "limit", "offset"), nil, "")
	case "identity-events":
		return c.request(inv, http.MethodGet, "/cloud/identity-events"+query(inv, "limit"), nil, "")
	case "contacts":
		if len(args) < 2 {
			return c.usage()
		}
		return c.request(inv, http.MethodGet, "/cloud/contacts/"+args[1], nil, "")
	case "business":
		return c.business(inv, args)
	case "api":
		return c.rawAPI(inv, args)
	case "coverage":
		return c.coverage(inv)
	default:
		return c.usage()
	}
}

func query(inv invocation, names ...string) string {
	var parts []string
	for _, name := range names {
		if v, ok := inv.vals[name]; ok && v != "" {
			parts = append(parts, name+"="+v)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "?" + strings.Join(parts, "&")
}

func (c *cli) send(inv invocation, args []string) int {
	if len(args) == 0 {
		return c.usage()
	}
	to := c.value(inv, "to", "", "")
	if to == "" {
		_, _ = fmt.Fprintln(c.stderr, "--to is required")
		return 2
	}
	switch args[0] {
	case "text":
		body, ct := jsonBody(map[string]any{"number": to, "text": c.value(inv, "text", "", "")})
		return c.request(inv, http.MethodPost, "/send/text", body, ct)
	case "template":
		payload := map[string]any{
			"number": to, "name": c.value(inv, "name", "", ""),
			"language": c.value(inv, "lang", "", "en_US"),
		}
		if raw := c.value(inv, "components", "", ""); raw != "" {
			var components any
			data := []byte(raw)
			if strings.HasPrefix(raw, "@") || strings.HasPrefix(raw, ".") {
				b, err := os.ReadFile(strings.TrimPrefix(raw, "@"))
				if err != nil {
					_, _ = fmt.Fprintln(c.stderr, err)
					return 2
				}
				data = b
			}
			if err := json.Unmarshal(data, &components); err != nil {
				_, _ = fmt.Fprintln(c.stderr, "invalid --components:", err)
				return 2
			}
			payload["components"] = components
		}
		body, ct := jsonBody(payload)
		return c.request(inv, http.MethodPost, "/send/template", body, ct)
	case "media":
		return c.sendMedia(inv, to)
	default:
		return c.usage()
	}
}

func (c *cli) sendMedia(inv invocation, to string) int {
	file := c.value(inv, "file", "", "")
	if file == "" {
		_, _ = fmt.Fprintln(c.stderr, "--file is required")
		return 2
	}
	kind := c.value(inv, "type", "", "image")
	fields := map[string]string{
		"number": to, "type": kind, "caption": c.value(inv, "caption", "", ""),
		"filename": filepath.Base(file),
	}
	if inv.flags["dry-run"] {
		_, _ = fmt.Fprintf(c.stdout, "DRY RUN POST %s/send/media\n", c.baseURL(inv))
		for _, k := range []string{"number", "type", "caption", "filename"} {
			if fields[k] != "" {
				_, _ = fmt.Fprintf(c.stdout, "%s: %s\n", k, fields[k])
			}
		}
		_, _ = fmt.Fprintf(c.stdout, "file: %s\n", file)
		return 0
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	f, err := os.Open(file)
	if err != nil {
		_, _ = fmt.Fprintln(c.stderr, err)
		return 2
	}
	defer f.Close()
	part, err := w.CreateFormFile("file", filepath.Base(file))
	if err != nil {
		_, _ = fmt.Fprintln(c.stderr, err)
		return 2
	}
	if _, err := io.Copy(part, f); err != nil {
		_, _ = fmt.Fprintln(c.stderr, err)
		return 2
	}
	_ = w.Close()
	return c.request(inv, http.MethodPost, "/send/media", body.Bytes(), w.FormDataContentType())
}

func (c *cli) templateBody(inv invocation) ([]byte, error) {
	if inv.vals["body"] != "" || inv.vals["file"] != "" {
		return c.readBody(inv)
	}
	payload := map[string]any{}
	if v := c.value(inv, "name", "", ""); v != "" {
		payload["name"] = v
	}
	if v := c.value(inv, "lang", "", ""); v != "" {
		payload["language"] = v
	} else if v := c.value(inv, "language", "", ""); v != "" {
		payload["language"] = v
	}
	if v := c.value(inv, "category", "", ""); v != "" {
		payload["category"] = v
	}
	if raw := c.value(inv, "components", "", ""); raw != "" {
		var components any
		if err := json.Unmarshal([]byte(raw), &components); err != nil {
			return nil, fmt.Errorf("invalid --components: %w", err)
		}
		payload["components"] = components
	} else if body := c.value(inv, "template-body", "", ""); body != "" {
		payload["components"] = []map[string]string{{"type": "BODY", "text": body}}
	}
	if len(payload) == 0 {
		return nil, errors.New("--body/--file or template flags are required")
	}
	return json.Marshal(payload)
}

func (c *cli) templates(inv invocation, args []string) int {
	if len(args) == 0 {
		return c.usage()
	}
	switch args[0] {
	case "list":
		return c.request(inv, http.MethodGet, "/cloud/templates", nil, "")
	case "sync":
		return c.request(inv, http.MethodPost, "/cloud/templates/sync", []byte("{}"), "application/json")
	case "create":
		body, err := c.templateBody(inv)
		if err != nil {
			_, _ = fmt.Fprintln(c.stderr, err)
			return 2
		}
		return c.request(inv, http.MethodPost, "/cloud/templates", body, "application/json")
	case "update":
		if len(args) < 3 {
			return c.usage()
		}
		body, err := c.templateBody(inv)
		if err != nil {
			_, _ = fmt.Fprintln(c.stderr, err)
			return 2
		}
		return c.request(inv, http.MethodPut, "/cloud/templates/"+args[1]+"/"+args[2], body, "application/json")
	case "delete":
		if len(args) < 3 {
			return c.usage()
		}
		if !inv.flags["yes"] && !inv.flags["dry-run"] {
			_, _ = fmt.Fprintln(c.stderr, "refusing to delete without --yes")
			return 2
		}
		return c.request(inv, http.MethodDelete, "/cloud/templates/"+args[1]+"/"+args[2], nil, "")
	default:
		return c.usage()
	}
}

func (c *cli) business(inv invocation, args []string) int {
	if len(args) == 0 {
		return c.usage()
	}
	switch args[0] {
	case "phone-numbers":
		return c.request(inv, http.MethodGet, "/cloud/business/phone-numbers", nil, "")
	case "waba":
		return c.request(inv, http.MethodGet, "/cloud/business/waba", nil, "")
	case "wabas":
		return c.request(inv, http.MethodGet, "/cloud/business/wabas"+query(inv, "scope"), nil, "")
	case "analytics":
		return c.request(inv, http.MethodGet, "/cloud/business/analytics"+query(inv, "start", "end", "granularity", "phoneNumbers"), nil, "")
	case "catalogs":
		if len(args) > 1 && args[1] == "create" {
			body, ct := jsonBody(map[string]string{"name": c.value(inv, "name", "", "")})
			return c.request(inv, http.MethodPost, "/cloud/business/catalogs", body, ct)
		}
		return c.request(inv, http.MethodGet, "/cloud/business/catalogs", nil, "")
	case "products":
		catalogID := c.value(inv, "catalog", "", "")
		if catalogID == "" {
			_, _ = fmt.Fprintln(c.stderr, "--catalog is required")
			return 2
		}
		path := "/cloud/business/catalogs/" + url.PathEscape(catalogID) + "/products"
		if len(args) > 1 && args[1] == "create" {
			body, err := c.readBody(inv)
			if err != nil {
				_, _ = fmt.Fprintln(c.stderr, err)
				return 2
			}
			return c.request(inv, http.MethodPost, path, body, "application/json")
		}
		return c.request(inv, http.MethodGet, path, nil, "")
	default:
		return c.usage()
	}
}

func (c *cli) rawAPI(inv invocation, args []string) int {
	if len(args) < 2 {
		return c.usage()
	}
	method, path := args[0], args[1]
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	var body []byte
	if raw := inv.vals["body"]; raw != "" {
		var err error
		body, err = c.readBody(inv)
		if err != nil {
			_, _ = fmt.Fprintln(c.stderr, err)
			return 2
		}
	}
	ct := "application/json"
	if strings.HasPrefix(path, "/v") || strings.Contains(path, "graph.facebook.com") {
		// A raw Graph call is an admin operation and needs the global key.
		return c.request(inv, method, path, body, ct)
	}
	return c.request(inv, method, path, body, ct)
}

func (c *cli) doctor(inv invocation) int {
	path := "/cloud/diagnostics"
	auth := c.value(inv, "instance-token", "WAMUX_INSTANCE_TOKEN", "")
	if auth == "" {
		if id := inv.vals["instance"]; id != "" {
			if admin := c.value(inv, "api-key", "WAMUX_API_KEY", ""); admin != "" {
				path = "/cloud/admin/instances/" + url.PathEscape(id) + "/diagnostics"
				auth = admin
			} else {
				path = "/server/health"
			}
		} else {
			path = "/server/health"
			auth = c.value(inv, "api-key", "WAMUX_API_KEY", "")
		}
	}
	r, err := c.do(inv, http.MethodGet, path, nil, "", auth)
	if err != nil {
		_, _ = fmt.Fprintln(c.stderr, err)
		return 1
	}
	if r == nil {
		return 0
	}
	ok := r.Status >= 200 && r.Status < 300
	var envelope struct {
		Data struct {
			TokenValid       bool   `json:"tokenValid"`
			AppSecretSet     bool   `json:"appSecretSet"`
			PhoneRegistered  bool   `json:"phoneRegistered"`
			Subscribed       bool   `json:"subscribed"`
			LastGraphError   string `json:"lastGraphError"`
			LastWebhookAt    string `json:"lastWebhookAt"`
			TemplateRejected int    `json:"templatesRejected"`
		} `json:"data"`
	}
	_ = json.Unmarshal(r.Body, &envelope)
	d := envelope.Data
	if path == "/cloud/diagnostics" {
		if !d.TokenValid || !d.AppSecretSet || !d.PhoneRegistered || !d.Subscribed || d.LastGraphError != "" {
			ok = false
		}
		if d.LastWebhookAt != "" {
			if at, err := time.Parse(time.RFC3339, d.LastWebhookAt); err == nil && time.Since(at) > 7*24*time.Hour {
				ok = false
			}
		}
	}
	if inv.flags["json"] {
		_, _ = fmt.Fprintln(c.stdout, strings.TrimSpace(string(r.Body)))
	} else {
		_, _ = fmt.Fprintf(c.stdout, "doctor: %s\n", map[bool]string{true: "healthy", false: "failed"}[ok])
		_, _ = fmt.Fprintf(c.stdout, "tokenValid=%t appSecretSet=%t phoneRegistered=%t subscribed=%t\n",
			d.TokenValid, d.AppSecretSet, d.PhoneRegistered, d.Subscribed)
		if d.LastWebhookAt != "" {
			_, _ = fmt.Fprintf(c.stdout, "lastWebhookAt=%s\n", d.LastWebhookAt)
		}
		if d.TemplateRejected > 0 {
			_, _ = fmt.Fprintf(c.stdout, "templatesRejected=%d\n", d.TemplateRejected)
		}
		if d.LastGraphError != "" {
			_, _ = fmt.Fprintf(c.stdout, "lastGraphError=%s\n", d.LastGraphError)
		}
	}
	if !ok {
		return 1
	}
	return 0
}

func (c *cli) coverage(inv invocation) int {
	type op struct {
		Operation string `json:"operation"`
		Endpoint  string `json:"endpoint"`
		Status    string `json:"status"`
	}
	ops := []op{
		{"send text", "POST /messages", "implemented"},
		{"send media", "POST /messages", "implemented"},
		{"send template", "POST /messages", "implemented"},
		{"templates", "GET/POST /message_templates", "implemented"},
		{"phone verify", "GET /{phone-number-id}", "implemented"},
		{"phone register", "POST /{phone-number-id}/register", "implemented"},
		{"subscribe app", "POST /{waba-id}/subscribed_apps", "implemented"},
		{"phone list", "GET /{waba-id}/phone_numbers", "implemented"},
		{"analytics", "GET /{waba-id}/analytics", "implemented"},
		{"catalogs", "GET/POST /{business-id}/owned_product_catalogs", "implemented"},
		{"embedded signup", "POST /cloud/embedded/start", "implemented"},
		{"groups", "POST /{phone-number-id}/groups", "separate"},
		{"calling", "POST /{phone-number-id}/calls", "separate"},
		{"flows", "POST /{phone-number-id}/flows", "separate"},
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].Operation < ops[j].Operation })
	if inv.flags["json"] {
		out, _ := json.Marshal(ops)
		_, _ = fmt.Fprintln(c.stdout, string(out))
		return 0
	}
	for _, o := range ops {
		_, _ = fmt.Fprintf(c.stdout, "%-22s %-40s %s\n", o.Operation, o.Endpoint, o.Status)
	}
	return 0
}

func (c *cli) usage() int {
	_, _ = fmt.Fprint(c.stderr, `wamuxctl `+version+`

Usage:
  wamuxctl cloud doctor [--instance-token TOKEN] [--json]
  wamuxctl cloud status|verify|register
  wamuxctl cloud credentials get|set
  wamuxctl cloud send text|media|template --to NUMBER ...
  wamuxctl cloud templates list|sync|create|update|delete
  wamuxctl cloud account-events list
  wamuxctl cloud business phone-numbers|waba|wabas|analytics|catalogs|products
  wamuxctl cloud contacts get|identities CONTACT_ID
  wamuxctl cloud api METHOD PATH [--body FILE]
  wamuxctl cloud coverage

Global flags: --url URL --api-key KEY --instance-token TOKEN --json
              --timeout 30s --verbose --dry-run --yes
Environment: WAMUX_URL, WAMUX_API_KEY, WAMUX_INSTANCE_TOKEN
`)
	return 2
}

func run(args []string, stdout, stderr io.Writer) int {
	c := &cli{stdout: stdout, stderr: stderr, client: &http.Client{Timeout: 30 * time.Second}}
	inv, err := parseArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	if inv.flags["version"] {
		_, _ = fmt.Fprintln(stdout, version)
		return 0
	}
	if inv.flags["help"] || len(inv.pos) == 0 {
		return c.usage()
	}
	c.client.Timeout = c.timeout(inv)
	switch inv.pos[0] {
	case "cloud":
		return c.cloud(inv)
	case "version":
		_, _ = fmt.Fprintln(stdout, version)
		return 0
	default:
		return c.usage()
	}
}
