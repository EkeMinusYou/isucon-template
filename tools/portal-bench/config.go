package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type config struct {
	BaseURL        string            `json:"base_url"`
	Vars           map[string]string `json:"vars"`
	Headers        map[string]string `json:"headers"`
	Timeout        string            `json:"timeout"`
	RequestTimeout string            `json:"request_timeout"`
	PollInterval   string            `json:"poll_interval"`
	PollRetries    int               `json:"poll_retries"`
	Start          action            `json:"start"`
	Poll           action            `json:"poll"`
	Result         *action           `json:"result,omitempty"`
	Outcome        outcome           `json:"outcome"`
}

// Each phase can use HTTP or an argv command that returns a response on stdout.
type action struct {
	HTTP     *httpAction       `json:"http,omitempty"`
	Command  []string          `json:"command,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Response response          `json:"response"`
}

type httpAction struct {
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
	BodyBase64 string            `json:"body_base64,omitempty"`
}

type response struct {
	Format string              `json:"format"`
	Fields map[string]selector `json:"fields"`
}

type selector struct {
	Path    string        `json:"path"`
	Type    string        `json:"type,omitempty"`
	Default *string       `json:"default,omitempty"`
	Trim    bool          `json:"trim,omitempty"`
	Replace []replacement `json:"replace,omitempty"`
}

type replacement struct {
	Pattern string `json:"pattern"`
	Value   string `json:"value"`
}

type outcome struct {
	Status       string   `json:"status"`
	DoneValues   []string `json:"done_values"`
	FailedValues []string `json:"failed_values,omitempty"`
	Pass         string   `json:"pass,omitempty"`
	PassValues   []string `json:"pass_values,omitempty"`
	FailValues   []string `json:"fail_values,omitempty"`
	Score        string   `json:"score,omitempty"`
	Logs         []string `json:"logs"`
}

func loadConfig(path string) (config, error) {
	f, err := os.Open(path)
	if err != nil {
		return config{}, err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	var c config
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("config must contain one JSON object")
	}
	if c.Timeout == "" {
		c.Timeout = "20m"
	}
	if c.RequestTimeout == "" {
		c.RequestTimeout = "30s"
	}
	if c.PollInterval == "" {
		c.PollInterval = "5s"
	}
	return c, nil
}

func positiveDuration(value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid positive duration %q", value)
	}
	return d, nil
}

type missingEnvironment struct{ key string }

func (e missingEnvironment) Error() string {
	return fmt.Sprintf("environment variable %s is required", e.key)
}

func render(s string, vars map[string]string) (string, error) {
	t, err := template.New("value").Option("missingkey=error").Funcs(template.FuncMap{
		"env": func(key string) (string, error) {
			v, ok := os.LookupEnv(key)
			if !ok || v == "" {
				return "", missingEnvironment{key}
			}
			return v, nil
		},
		"urlquery": url.QueryEscape,
		"json":     func(v string) string { b, _ := json.Marshal(v); return string(b) },
		"base64":   func(v string) string { return base64.StdEncoding.EncodeToString([]byte(v)) },
		"protoUint": func(field int, value string) (string, error) {
			if field <= 0 || field >= 1<<29 {
				return "", fmt.Errorf("invalid field number")
			}
			n, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return "", fmt.Errorf("invalid unsigned integer")
			}
			b := binary.AppendUvarint(nil, uint64(field)<<3)
			b = binary.AppendUvarint(b, n)
			return base64.StdEncoding.EncodeToString(b), nil
		},
	}).Parse(s)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	err = t.Execute(&b, vars)
	// Template errors can include argument values; never expose credentials.
	if err != nil {
		var missing missingEnvironment
		if errors.As(err, &missing) {
			return "", missing
		}
		return "", fmt.Errorf("template evaluation failed; check variables and required environment variables")
	}
	return b.String(), nil
}

func (c config) variables() (map[string]string, error) {
	v := make(map[string]string)
	for k, value := range c.Vars {
		if k == "base_url" || k == "job_id" {
			return nil, fmt.Errorf("reserved variable %s", k)
		}
		resolved, err := render(value, nil)
		if err != nil {
			return nil, fmt.Errorf("variable %s: %w", k, err)
		}
		v[k] = resolved
	}
	base, err := render(c.BaseURL, v)
	if err != nil {
		return nil, err
	}
	if base != "" {
		u, err := url.Parse(base)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return nil, fmt.Errorf("base_url must be an HTTP(S) URL without userinfo")
		}
	}
	v["base_url"] = strings.TrimRight(base, "/")
	return v, nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (c config) validate() error {
	for _, s := range []string{c.Timeout, c.RequestTimeout, c.PollInterval} {
		if _, err := positiveDuration(s); err != nil {
			return err
		}
	}
	if c.PollRetries < 0 {
		return fmt.Errorf("poll_retries must be nonnegative")
	}
	v, err := c.variables()
	if err != nil {
		return err
	}
	v["job_id"] = "1"
	for _, a := range []action{c.Start, c.Poll} {
		for name := range a.Response.Fields {
			v[name] = "1"
		}
	}
	for name, a := range map[string]action{"start": c.Start, "poll": c.Poll} {
		if err := validateAction(a, c.Headers, v); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if _, ok := c.Start.Response.Fields["job_id"]; !ok {
		return fmt.Errorf("start must extract job_id")
	}
	if c.Outcome.Status == "" || len(c.Outcome.DoneValues) == 0 || len(c.Outcome.Logs) == 0 {
		return fmt.Errorf("outcome requires status, done_values, and logs")
	}
	if _, ok := c.Poll.Response.Fields[c.Outcome.Status]; !ok {
		return fmt.Errorf("poll must extract outcome.status")
	}
	fields := c.Poll.Response.Fields
	if c.Result != nil {
		if err := validateAction(*c.Result, c.Headers, v); err != nil {
			return fmt.Errorf("result: %w", err)
		}
		fields = make(map[string]selector)
		for k, s := range c.Poll.Response.Fields {
			fields[k] = s
		}
		for k, s := range c.Result.Response.Fields {
			fields[k] = s
		}
	}
	for _, k := range append(append([]string{}, c.Outcome.Logs...), c.Outcome.Score, c.Outcome.Pass) {
		if k != "" {
			if _, ok := fields[k]; !ok {
				return fmt.Errorf("outcome field %s is not extracted", k)
			}
		}
	}
	if c.Outcome.Pass != "" && len(c.Outcome.PassValues)+len(c.Outcome.FailValues) == 0 {
		return fmt.Errorf("outcome.pass requires pass_values or fail_values")
	}
	for _, value := range c.Outcome.PassValues {
		if contains(c.Outcome.FailValues, value) {
			return fmt.Errorf("overlapping pass/fail values")
		}
	}
	return nil
}

func validateAction(a action, headers map[string]string, vars map[string]string) error {
	if (a.HTTP == nil) == (len(a.Command) == 0) {
		return fmt.Errorf("specify exactly one of http or command")
	}
	if !contains([]string{"json", "protobuf", "text", "regex"}, a.Response.Format) {
		return fmt.Errorf("response.format must be json, protobuf, text, or regex")
	}
	for name, s := range a.Response.Fields {
		if name == "base_url" {
			return fmt.Errorf("cannot extract reserved field base_url")
		}
		if a.Response.Format == "protobuf" {
			if _, err := parseProtoPath(s.Path); err != nil {
				return err
			}
			if !contains([]string{"", "auto", "string", "bytes", "uint", "int", "sint", "bool", "float32", "float64"}, s.Type) {
				return fmt.Errorf("unsupported protobuf type %s", s.Type)
			}
		} else if a.Response.Format == "json" && s.Path != "" && !strings.HasPrefix(s.Path, "/") {
			return fmt.Errorf("JSON paths must be JSON pointers")
		}
		if a.Response.Format == "regex" {
			r, err := regexp.Compile(s.Path)
			if err != nil || r.NumSubexp() != 1 {
				return fmt.Errorf("regex field %s requires a valid pattern with one capture group", name)
			}
		}
		for _, replace := range s.Replace {
			if _, err := regexp.Compile(replace.Pattern); err != nil {
				return fmt.Errorf("invalid replacement pattern for %s", name)
			}
		}
	}
	values := append([]string{}, a.Command...)
	if len(a.Command) > 0 {
		program, err := render(a.Command[0], vars)
		if err != nil {
			return err
		}
		if _, err := exec.LookPath(program); err != nil {
			return fmt.Errorf("configured command executable is unavailable")
		}
	}
	for _, value := range a.Env {
		values = append(values, value)
	}
	if a.HTTP != nil {
		h := a.HTTP
		if h.Method == "" || h.URL == "" {
			return fmt.Errorf("http requires method and url")
		}
		if h.Body != "" && h.BodyBase64 != "" {
			return fmt.Errorf("body and body_base64 are mutually exclusive")
		}
		values = append(values, h.URL, h.Body, h.BodyBase64)
		for _, value := range headers {
			values = append(values, value)
		}
		for _, value := range h.Headers {
			values = append(values, value)
		}
		if _, err := makeRequest(context.Background(), *h, headers, vars); err != nil {
			return err
		}
		if h.BodyBase64 != "" {
			s, err := render(h.BodyBase64, vars)
			if err != nil {
				return err
			}
			if _, err := base64.StdEncoding.DecodeString(s); err != nil {
				return fmt.Errorf("invalid body_base64")
			}
		}
	}
	for _, value := range values {
		if _, err := render(value, vars); err != nil {
			return err
		}
	}
	return nil
}

func scalarString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case json.Number:
		return x.String(), nil
	case bool:
		return strconv.FormatBool(x), nil
	default:
		return "", fmt.Errorf("selected value must be a string, number, or boolean")
	}
}
