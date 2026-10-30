package main

import (
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"
)

// Readiness is an explicit deployment condition, not a business-state model.
func readinessScript(ready readiness, expand expander) (string, error) {
	address, err := expand.expand(ready.URL)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("ready.url must be an HTTP or HTTPS URL")
	}
	duration := 30 * time.Second
	if ready.Timeout != "" {
		text, err := expand.expand(ready.Timeout)
		if err != nil {
			return "", err
		}
		duration, err = time.ParseDuration(text)
		if err != nil || duration <= 0 {
			return "", fmt.Errorf("ready.timeout must be a positive duration")
		}
	}
	status := ready.Status
	if status == 0 {
		status = 200
	}
	if status < 100 || status > 599 {
		return "", fmt.Errorf("ready.status must be an HTTP status")
	}
	quoted := "'" + strings.ReplaceAll(address, "'", "'\"'\"'") + "'"
	return fmt.Sprintf(`set -eu
ready_deadline=$(( $(date +%%s) + %d ))
ready_status=unobserved
while :; do
  ready_remaining=$(( ready_deadline - $(date +%%s) ))
  if [ "$ready_remaining" -le 0 ]; then
    echo "application readiness timed out (last HTTP status: $ready_status)" >&2
    exit 1
  fi
  ready_status=$(curl --silent --max-time "$ready_remaining" --output /dev/null --write-out '%%{http_code}' %s 2>/dev/null || true)
  if [ "$ready_status" = '%d' ]; then break; fi
  sleep 0.2
done
`, int64(math.Ceil(duration.Seconds())), quoted, status), nil
}
