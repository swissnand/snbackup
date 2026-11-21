package repo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"hash/fnv"
	"path"
	"strings"
)

// Node is one entry (file, directory, symlink) of a snapshot tree.
// Paths are stored slash separated and relative, e.g. "etc/nginx/nginx.conf"
// or "C/Users/Public/file.txt" for Windows drive letters.
type Node struct {
	Path   string   `json:"p"`
	Type   string   `json:"t"` // f, d, l
	Mode   uint32   `json:"m"`
	MTime  int64    `json:"mt"`
	Size   int64    `json:"s,omitempty"`
	UID    int      `json:"u,omitempty"`
	GID    int      `json:"g,omitempty"`
	Link   string   `json:"l,omitempty"`
	Chunks []string `json:"c,omitempty"`
	Hash   string   `json:"h,omitempty"` // SHA-256 of file content
	Inode  uint64   `json:"i,omitempty"`
}

// treeWriter streams nodes as JSON lines into deduplicated blobs. Blob
// boundaries are content defined at line granularity so unchanged parts of a
// tree deduplicate between snapshots.
type treeWriter struct {
	r      *Repo
	ctx    context.Context
	buf    bytes.Buffer
	ids    []string
	stored int64
}

func (t *treeWriter) Add(n *Node) error {
	line, err := json.Marshal(n)
	if err != nil {
		return err
	}
	t.buf.Write(line)
	t.buf.WriteByte('\n')
	h := fnv.New32a()
	h.Write(line)
	if (t.buf.Len() >= 64<<10 && h.Sum32()&0x3f == 0) || t.buf.Len() >= 4<<20 {
		return t.flush()
	}
	return nil
}

func (t *treeWriter) flush() error {
	if t.buf.Len() == 0 {
		return nil
	}
	data := append([]byte(nil), t.buf.Bytes()...)
	t.buf.Reset()
	id, n, err := t.r.SaveBlob(t.ctx, data)
	if err != nil {
		return err
	}
	t.stored += n
	t.ids = append(t.ids, id)
	return nil
}

func (t *treeWriter) Close() ([]string, error) {
	return t.ids, t.flush()
}

// WalkTree calls fn for every node of a snapshot in stored order.
func (r *Repo) WalkTree(ctx context.Context, s *Snapshot, fn func(*Node) error) error {
	for _, id := range s.Tree {
		data, err := r.LoadBlob(ctx, id)
		if err != nil {
			return err
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var n Node
			if err := json.Unmarshal(sc.Bytes(), &n); err != nil {
				return err
			}
			if err := fn(&n); err != nil {
				return err
			}
		}
		if err := sc.Err(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

// LoadTree returns all nodes of a snapshot.
func (r *Repo) LoadTree(ctx context.Context, s *Snapshot) ([]*Node, error) {
	var out []*Node
	err := r.WalkTree(ctx, s, func(n *Node) error { out = append(out, n); return nil })
	return out, err
}

// ListDir returns the direct children of dir ("" = root) in a node list.
func ListDir(nodes []*Node, dir string) []*Node {
	dir = strings.Trim(dir, "/")
	seen := map[string]*Node{}
	var order []string
	for _, n := range nodes {
		p := n.Path
		var rest string
		if dir == "" {
			rest = p
		} else if strings.HasPrefix(p, dir+"/") {
			rest = p[len(dir)+1:]
		} else {
			continue
		}
		if rest == "" {
			continue
		}
		first := rest
		if i := strings.Index(rest, "/"); i >= 0 {
			first = rest[:i]
		}
		full := first
		if dir != "" {
			full = dir + "/" + first
		}
		if _, ok := seen[full]; ok {
			if full == p {
				seen[full] = n
			}
			continue
		}
		if full == p {
			seen[full] = n
		} else {
			seen[full] = &Node{Path: full, Type: "d"}
		}
		order = append(order, full)
	}
	out := make([]*Node, 0, len(order))
	for _, k := range order {
		out = append(out, seen[k])
	}
	return out
}

// safeRel reports whether a stored path is safe to restore (no traversal).
func safeRel(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\x00") {
		return false
	}
	c := path.Clean(p)
	return c == p && c != ".." && !strings.HasPrefix(c, "../")
}
