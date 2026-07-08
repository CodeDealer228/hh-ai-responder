package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HHRequester struct {
	ctx       context.Context
	client    *http.Client
	interval  time.Duration
	mu        sync.Mutex
	lastStart time.Time
}

func NewHHRequester(ctx context.Context, client *http.Client, interval time.Duration) *HHRequester {
	return &HHRequester{
		ctx:      ctx,
		client:   client,
		interval: interval,
	}
}

func (r *HHRequester) Do(req *http.Request) (*HHResponse, error) {
	// Rate limiting
	r.mu.Lock()
	if !r.lastStart.IsZero() {
		wait := time.Until(r.lastStart.Add(r.interval))
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-r.ctx.Done():
				timer.Stop()
				r.mu.Unlock()
				return nil, r.ctx.Err()
			}
		}
	}
	r.lastStart = time.Now()
	r.mu.Unlock()

	// Execute request
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	logger.Debug("%d %s %s", resp.StatusCode, req.Method, req.URL.String())

	return &HHResponse{
		Status: resp.StatusCode,
		URL:    req.URL,
		Body:   body,
	}, nil
}

func generateUUIDv4() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	), nil
}

type MemoryPersistentJar struct {
	mu          sync.Mutex
	cookies     map[string][]*http.Cookie
	persistPath string
}

func cookieEqual(a, b *http.Cookie) bool {
	return a.Name == b.Name &&
		a.Value == b.Value &&
		a.Path == b.Path &&
		a.Domain == b.Domain &&
		a.Secure == b.Secure &&
		a.Expires.Equal(b.Expires)
}

func NewMemoryPersistentJar(cookiesPath string) (*MemoryPersistentJar, error) {
	jar := &MemoryPersistentJar{
		cookies:     make(map[string][]*http.Cookie),
		persistPath: cookiesPath,
	}

	data, err := os.ReadFile(cookiesPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return jar, nil
		}
		return nil, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 7 {
			parts = strings.Fields(line)
		}
		if len(parts) < 7 {
			continue
		}

		domain := parts[0]
		expiresUnix, _ := strconv.ParseInt(parts[4], 10, 64)

		cookie := &http.Cookie{
			Domain: domain,
			Path:   parts[2],
			Secure: strings.EqualFold(parts[3], "TRUE"),
			Name:   parts[5],
			Value:  parts[6],
		}

		if expiresUnix > 0 {
			cookie.Expires = time.Unix(expiresUnix, 0)
		}

		jar.cookies[domain] = append(jar.cookies[domain], cookie)
	}

	return jar, scanner.Err()
}

func (j *MemoryPersistentJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()

	host := u.Hostname()
	changed := false

	for _, cookie := range cookies {
		domain := cookie.Domain
		if domain == "" {
			domain = host
		}

		var updated []*http.Cookie
		exists := false

		for _, c := range j.cookies[domain] {
			if c.Name == cookie.Name && c.Path == cookie.Path {
				exists = true

				if cookie.Expires.IsZero() && !c.Expires.IsZero() {
					cookie.Expires = c.Expires
				}

				if cookieEqual(c, cookie) {
					updated = append(updated, c)
				} else {
					updated = append(updated, cookie)
					changed = true
				}
			} else {
				updated = append(updated, c)
			}
		}

		if !exists {
			updated = append(updated, cookie)
			changed = true
		}

		j.cookies[domain] = updated
	}

	if changed && j.persistPath != "" {
		_ = j.saveLockedTo(j.persistPath)
	}
}

func (j *MemoryPersistentJar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()

	var matched []*http.Cookie
	host := u.Hostname()
	now := time.Now()
	changed := false

	for domain, list := range j.cookies {
		if domain == host ||
			(strings.HasPrefix(domain, ".") && strings.HasSuffix(host, domain)) ||
			strings.HasSuffix(host, "."+domain) {

			var active []*http.Cookie

			for _, cookie := range list {
				if !cookie.Expires.IsZero() && cookie.Expires.Before(now) {
					changed = true
					continue
				}

				if cookie.Secure && u.Scheme != "https" {
					continue
				}

				copied := *cookie
				matched = append(matched, &copied)
				active = append(active, cookie)
			}

			if len(active) != len(list) {
				j.cookies[domain] = active
			}
		}
	}

	if changed && j.persistPath != "" {
		_ = j.saveLockedTo(j.persistPath)
	}

	return matched
}

func (j *MemoryPersistentJar) Save(path string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.saveLockedTo(path)
}

func (j *MemoryPersistentJar) saveLockedTo(path string) error {
	if path == "" {
		return nil
	}

	var buffer bytes.Buffer

	buffer.WriteString("# Netscape HTTP Cookie File\n")
	buffer.WriteString("# http://curl.haxx.se/rfc/cookie_spec.html\n")
	buffer.WriteString("# This is a generated file! Do not edit.\n\n")

	for domain, list := range j.cookies {
		for _, cookie := range list {
			if cookie.Name == "" {
				continue
			}

			expires := int64(0)
			if !cookie.Expires.IsZero() {
				expires = cookie.Expires.Unix()
			}

			secure := "FALSE"
			if cookie.Secure {
				secure = "TRUE"
			}

			cookiePath := cookie.Path
			if cookiePath == "" {
				cookiePath = "/"
			}

			row := []string{
				domain,
				"TRUE",
				cookiePath,
				secure,
				strconv.FormatInt(expires, 10),
				cookie.Name,
				cookie.Value,
			}

			buffer.WriteString(strings.Join(row, "\t"))
			buffer.WriteByte('\n')
		}
	}

	tmpPath := path + "~"

	if err := os.WriteFile(tmpPath, buffer.Bytes(), 0o600); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func decodeEmbeddedJSON[T any](data []byte, marker string, out *T) error {
	_, after, ok := bytes.Cut(data, []byte(marker))
	if !ok {
		return fmt.Errorf("marker %q not found in response", marker)
	}

	var raw json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(after))
	if err := decoder.Decode(&raw); err != nil {
		return err
	}

	return json.Unmarshal(raw, out)
}

func cloneValues(values url.Values) url.Values {
	result := make(url.Values, len(values))
	for key, list := range values {
		result[key] = append([]string(nil), list...)
	}
	return result
}

func unexpectedHTTPStatus(status int) error {
	return fmt.Errorf("unexpected HTTP status %d %s", status, http.StatusText(status))
}
