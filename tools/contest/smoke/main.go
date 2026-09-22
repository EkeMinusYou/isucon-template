// Command smoke exercises a few requests against the application without
// running the benchmark, so that `task setup-smoke` can check that the
// collectors, logs, and profiles produce artifacts.
//
// Placeholder: task setup-smoke builds this for TARGET_OS/TARGET_ARCH, copies
// the binary to ENTRY_HOST, and runs it there with no arguments. Replace the
// flow below with the contest's own session flow (registration, login, and a
// few authenticated reads) during setup. Keep it read-mostly, keep it short,
// and never print session identifiers or credentials.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

func main() {
	base := flag.String("base", "http://127.0.0.1", "application base URL, reached from the host this runs on")
	flag.Parse()

	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatal(err)
	}
	// The jar keeps the session across steps once the flow logs in.
	c := &client{http: &http.Client{Jar: jar, Timeout: 15 * time.Second}, base: strings.TrimSuffix(*base, "/")}

	paths := []string{"/"}
	for _, path := range paths {
		if _, err := c.get(path); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("Smoke: %d unauthenticated read(s) passed; replace this flow with the contest's own\n", len(paths))
}

type client struct {
	http *http.Client
	base string
}

func (c *client) get(path string) ([]byte, error) {
	return c.do(http.MethodGet, path, nil)
}

func (c *client) do(method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	if res.StatusCode != http.StatusOK {
		// Response bodies can carry identifiers, so report the status only.
		return nil, fmt.Errorf("%s %s: status %d", method, path, res.StatusCode)
	}
	return data, nil
}
