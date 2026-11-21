package backend

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WebDAV stores objects on a WebDAV server (Nextcloud, ownCloud,
// Infomaniak kDrive, Synology WebDAV Server, Apache mod_dav ...).
type WebDAV struct {
	base       *url.URL
	user, pass string
	client     *http.Client
	mu         sync.Mutex
	madeDirs   map[string]bool
	baseOnce   sync.Once
	baseErr    error
}

// ensureBase creates the collection chain of the base URL itself.
func (w *WebDAV) ensureBase(ctx context.Context) error {
	w.baseOnce.Do(func() {
		cur := ""
		for _, p := range strings.Split(strings.Trim(w.base.Path, "/"), "/") {
			if p == "" {
				continue
			}
			cur += "/" + p
			u := *w.base
			u.Path = cur + "/"
			r, err := http.NewRequestWithContext(ctx, "MKCOL", u.String(), nil)
			if err != nil {
				w.baseErr = err
				return
			}
			if w.user != "" {
				r.SetBasicAuth(w.user, w.pass)
			}
			resp, err := w.client.Do(r)
			if err != nil {
				w.baseErr = err
				return
			}
			drain(resp.Body)
		}
	})
	return w.baseErr
}

// NewWebDAV creates a WebDAV backend.
func NewWebDAV(rawURL, user, pass string) (*WebDAV, error) {
	u, err := url.Parse(strings.TrimRight(rawURL, "/") + "/")
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
		return nil, errors.New("webdav: only https URLs are allowed (except localhost)")
	}
	return &WebDAV{base: u, user: user, pass: pass, client: &http.Client{Timeout: 10 * time.Minute}, madeDirs: map[string]bool{}}, nil
}

func (w *WebDAV) url(name string) string {
	u := *w.base
	u.Path = path.Join(w.base.Path, name)
	if strings.HasSuffix(name, "/") {
		u.Path += "/"
	}
	return u.String()
}

func (w *WebDAV) req(ctx context.Context, method, name string, body []byte, hdr map[string]string) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	r, err := http.NewRequestWithContext(ctx, method, w.url(name), rd)
	if err != nil {
		return nil, err
	}
	if w.user != "" {
		r.SetBasicAuth(w.user, w.pass)
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	return w.client.Do(r)
}

func (w *WebDAV) mkdirs(ctx context.Context, dir string) error {
	if err := w.ensureBase(ctx); err != nil {
		return err
	}
	parts := strings.Split(strings.Trim(dir, "/"), "/")
	cur := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		cur += p + "/"
		w.mu.Lock()
		done := w.madeDirs[cur]
		w.mu.Unlock()
		if done {
			continue
		}
		resp, err := w.req(ctx, "MKCOL", cur, nil, nil)
		if err != nil {
			return err
		}
		drain(resp.Body)
		// 201 created, 405 already exists
		if resp.StatusCode != 201 && resp.StatusCode != 405 && resp.StatusCode/100 != 2 {
			return fmt.Errorf("webdav: MKCOL %s: HTTP %d", cur, resp.StatusCode)
		}
		w.mu.Lock()
		w.madeDirs[cur] = true
		w.mu.Unlock()
	}
	return nil
}

func (w *WebDAV) Save(ctx context.Context, name string, data []byte) error {
	if err := w.mkdirs(ctx, path.Dir(name)); err != nil {
		return err
	}
	resp, err := w.req(ctx, http.MethodPut, name, data, map[string]string{"Content-Type": "application/octet-stream"})
	if err != nil {
		return err
	}
	drain(resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("webdav: PUT %s: HTTP %d", name, resp.StatusCode)
	}
	return nil
}

func (w *WebDAV) Load(ctx context.Context, name string) ([]byte, error) {
	resp, err := w.req(ctx, http.MethodGet, name, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, ErrNotFound
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("webdav: GET %s: HTTP %d", name, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (w *WebDAV) Exists(ctx context.Context, name string) (bool, error) {
	resp, err := w.req(ctx, http.MethodHead, name, nil, nil)
	if err != nil {
		return false, err
	}
	drain(resp.Body)
	if resp.StatusCode == 404 {
		return false, nil
	}
	return resp.StatusCode/100 == 2, nil
}

func (w *WebDAV) Delete(ctx context.Context, name string) error {
	resp, err := w.req(ctx, http.MethodDelete, name, nil, nil)
	if err != nil {
		return err
	}
	drain(resp.Body)
	if resp.StatusCode/100 != 2 && resp.StatusCode != 404 {
		return fmt.Errorf("webdav: DELETE %s: HTTP %d", name, resp.StatusCode)
	}
	return nil
}

type davMultistatus struct {
	Responses []struct {
		Href string `xml:"href"`
		Prop struct {
			Length       string `xml:"getcontentlength"`
			ResourceType struct {
				Collection *struct{} `xml:"collection"`
			} `xml:"resourcetype"`
		} `xml:"propstat>prop"`
	} `xml:"response"`
}

func (w *WebDAV) List(ctx context.Context, dir string) ([]FileInfo, error) {
	var out []FileInfo
	var walk func(d string) error
	walk = func(d string) error {
		body := []byte(`<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/><d:getcontentlength/></d:prop></d:propfind>`)
		resp, err := w.req(ctx, "PROPFIND", strings.TrimSuffix(d, "/")+"/", body, map[string]string{"Depth": "1", "Content-Type": "application/xml"})
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode == 404 {
			return nil
		}
		if resp.StatusCode != 207 {
			return fmt.Errorf("webdav: PROPFIND %s: HTTP %d", d, resp.StatusCode)
		}
		var ms davMultistatus
		if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
			return err
		}
		selfPath := path.Join(w.base.Path, d)
		for _, r := range ms.Responses {
			hu, err := url.Parse(r.Href)
			if err != nil {
				continue
			}
			p := strings.TrimSuffix(hu.Path, "/")
			if p == strings.TrimSuffix(selfPath, "/") {
				continue
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(p, strings.TrimSuffix(w.base.Path, "/")), "/")
			if r.Prop.ResourceType.Collection != nil {
				if err := walk(rel); err != nil {
					return err
				}
				continue
			}
			size, _ := strconv.ParseInt(r.Prop.Length, 10, 64)
			out = append(out, FileInfo{Name: rel, Size: size})
		}
		return nil
	}
	return out, walk(strings.Trim(dir, "/"))
}

func (w *WebDAV) Location() string { return "webdav:" + w.base.Redacted() }
func (w *WebDAV) Close() error     { return nil }
