package api

import (
	"net/http"
	"sync"
	"time"
)

const readCacheEntries = 128
const readCacheBytes = 16 << 20
const readCacheItemBytes = 1 << 20

type cachedRead struct {
	header  http.Header
	body    []byte
	expires time.Time
}

type readCache struct {
	mu         sync.Mutex
	entries    map[string]cachedRead
	pending    map[string]chan struct{}
	bytes      int
	generation uint64
}

func newReadCache() *readCache {
	return &readCache{entries: make(map[string]cachedRead), pending: make(map[string]chan struct{})}
}

func (c *readCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.entries = make(map[string]cachedRead)
	c.bytes = 0
}

// This handler is INSIDE the host middleware: every cache hit still passes
// current authentication, permissions, API key quotas and durable accounting.
// Approved reviews use this same handler and therefore invalidate it as well.
func (a *API) serveCachedContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response := &bufferedResponse{header: make(http.Header)}
		a.serveContent(response, r)
		// A 5xx may follow a committed mutation (for example an audit
		// completion failure), so it must not leave an older cached response.
		if response.status >= 200 && response.status < 300 || response.status >= 500 {
			a.cache.invalidate()
		}
		replayResponse(w, response)
		return
	}
	key := r.URL.RequestURI()
	c := a.cache
	for {
		c.mu.Lock()
		if hit, ok := c.entries[key]; ok {
			if time.Now().Before(hit.expires) {
				c.mu.Unlock()
				for name, values := range hit.header {
					w.Header()[name] = append([]string(nil), values...)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(hit.body)
				return
			}
			delete(c.entries, key)
			c.bytes -= len(hit.body)
		}
		if waiting, ok := c.pending[key]; ok {
			c.mu.Unlock()
			select {
			case <-waiting:
				continue
			case <-r.Context().Done():
				return
			}
		}
		// Bound both stored responses and concurrent unique cache fills.
		if len(c.pending) >= readCacheEntries {
			c.mu.Unlock()
			a.serveContent(w, r)
			return
		}
		waiting := make(chan struct{})
		c.pending[key] = waiting
		generation := c.generation
		c.mu.Unlock()
		func() {
			defer func() {
				c.mu.Lock()
				delete(c.pending, key)
				close(waiting)
				c.mu.Unlock()
			}()
			response := &bufferedResponse{header: make(http.Header)}
			a.serveContent(response, r)
			if response.status == http.StatusOK && response.body.Len() <= readCacheItemBytes && r.Context().Err() == nil {
				c.mu.Lock()
				if c.generation == generation {
					// Evict the entry expiring first; memory remains bounded.
					for len(c.entries) >= readCacheEntries || c.bytes+response.body.Len() > readCacheBytes {
						var oldest string
						var expiry time.Time
						for name, entry := range c.entries {
							if expiry.IsZero() || entry.expires.Before(expiry) {
								oldest, expiry = name, entry.expires
							}
						}
						c.bytes -= len(c.entries[oldest].body)
						delete(c.entries, oldest)
					}
					ttl := 30 * time.Second
					if r.URL.Path == "/api/v2/tags" || r.URL.Path == "/api/v2/platforms" {
						ttl = 5 * time.Minute
					}
					body := append([]byte(nil), response.body.Bytes()...)
					c.entries[key] = cachedRead{header: response.header.Clone(), body: body, expires: time.Now().Add(ttl)}
					c.bytes += len(body)
				}
				c.mu.Unlock()
			}
			replayResponse(w, response)
		}()
		return
	}
}

func replayResponse(w http.ResponseWriter, response *bufferedResponse) {
	for name, values := range response.header {
		w.Header()[name] = append([]string(nil), values...)
	}
	status := response.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(response.body.Bytes())
}
