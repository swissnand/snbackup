// Package archive creates self-contained tar.gz archives, optionally
// encrypted (".snarc": AES-256-GCM in authenticated 1 MiB segments with
// truncation protection, key derived via PBKDF2-SHA256).
package archive

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	magic    = "SNARC001"
	segSize  = 1 << 20
	kdfIters = 600_000
)

// encWriter encrypts a stream in segments.
type encWriter struct {
	w       io.Writer
	gcm     cipher.AEAD
	prefix  [4]byte
	counter uint64
	buf     []byte
}

func newEncWriter(w io.Writer, password string) (*encWriter, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, kdfIters, 32)
	if err != nil {
		return nil, err
	}
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	e := &encWriter{w: w, gcm: gcm, buf: make([]byte, 0, segSize)}
	if _, err := io.ReadFull(rand.Reader, e.prefix[:]); err != nil {
		return nil, err
	}
	hdr := append([]byte(magic), salt...)
	var it [4]byte
	binary.BigEndian.PutUint32(it[:], kdfIters)
	hdr = append(hdr, it[:]...)
	hdr = append(hdr, e.prefix[:]...)
	_, err = w.Write(hdr)
	return e, err
}

func (e *encWriter) nonce() []byte {
	n := make([]byte, 12)
	copy(n, e.prefix[:])
	binary.BigEndian.PutUint64(n[4:], e.counter)
	return n
}

func (e *encWriter) flush(final bool) error {
	aad := []byte{0}
	if final {
		aad[0] = 1
	}
	ct := e.gcm.Seal(nil, e.nonce(), e.buf, aad)
	e.counter++
	var l [5]byte
	l[0] = aad[0]
	binary.BigEndian.PutUint32(l[1:], uint32(len(ct)))
	if _, err := e.w.Write(l[:]); err != nil {
		return err
	}
	_, err := e.w.Write(ct)
	e.buf = e.buf[:0]
	return err
}

func (e *encWriter) Write(p []byte) (int, error) {
	n := 0
	for len(p) > 0 {
		space := segSize - len(e.buf)
		if space > len(p) {
			space = len(p)
		}
		e.buf = append(e.buf, p[:space]...)
		p = p[space:]
		n += space
		if len(e.buf) == segSize {
			if err := e.flush(false); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

func (e *encWriter) Close() error { return e.flush(true) }

type decReader struct {
	r       io.Reader
	gcm     cipher.AEAD
	prefix  [4]byte
	counter uint64
	buf     []byte
	done    bool
}

func newDecReader(r io.Reader, password string) (*decReader, error) {
	hdr := make([]byte, len(magic)+16+4+4)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}
	if string(hdr[:len(magic)]) != magic {
		return nil, errors.New("keine verschlüsselte SNARC-Datei")
	}
	salt := hdr[8:24]
	iters := int(binary.BigEndian.Uint32(hdr[24:28]))
	if iters < 10000 {
		return nil, errors.New("ungültiger Header")
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, iters, 32)
	if err != nil {
		return nil, err
	}
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	d := &decReader{r: r, gcm: gcm}
	copy(d.prefix[:], hdr[28:32])
	return d, nil
}

func (d *decReader) Read(p []byte) (int, error) {
	for len(d.buf) == 0 {
		if d.done {
			return 0, io.EOF
		}
		var l [5]byte
		if _, err := io.ReadFull(d.r, l[:]); err != nil {
			return 0, errors.New("Archiv abgeschnitten oder beschädigt")
		}
		size := binary.BigEndian.Uint32(l[1:])
		if size > segSize+64 {
			return 0, errors.New("ungültiges Segment")
		}
		ct := make([]byte, size)
		if _, err := io.ReadFull(d.r, ct); err != nil {
			return 0, errors.New("Archiv abgeschnitten oder beschädigt")
		}
		n := make([]byte, 12)
		copy(n, d.prefix[:])
		binary.BigEndian.PutUint64(n[4:], d.counter)
		d.counter++
		pt, err := d.gcm.Open(nil, n, ct, []byte{l[0]})
		if err != nil {
			return 0, errors.New("falsches Passwort oder manipuliertes Archiv")
		}
		d.buf = pt
		d.done = l[0] == 1
	}
	n := copy(p, d.buf)
	d.buf = d.buf[n:]
	return n, nil
}

// Matcher decides whether a path (slash form, no leading slash) is excluded.
type Matcher interface{ Match(string) bool }

// Create writes a tar.gz (optionally encrypted) of the given paths.
func Create(out string, paths []string, ex Matcher, password string, log func(string)) (files int64, bytes int64, err error) {
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return 0, 0, err
	}
	tmp := out + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(tmp)
		}
	}()
	bw := bufio.NewWriterSize(f, 1<<20)
	var w io.Writer = bw
	var enc *encWriter
	if password != "" {
		if enc, err = newEncWriter(bw, password); err != nil {
			return
		}
		w = enc
	}
	gz, _ := gzip.NewWriterLevel(w, gzip.DefaultCompression)
	tw := tar.NewWriter(gz)
	for _, root := range paths {
		abs, e := filepath.Abs(root)
		if e != nil {
			return files, bytes, e
		}
		e = filepath.WalkDir(abs, func(p string, d fs.DirEntry, werr error) error {
			if werr != nil {
				if log != nil {
					log("Übersprungen: " + werr.Error())
				}
				return nil
			}
			name := strings.TrimPrefix(filepath.ToSlash(p), "/")
			if vol := filepath.VolumeName(p); vol != "" {
				name = strings.TrimSuffix(vol, ":") + filepath.ToSlash(strings.TrimPrefix(p, vol))
			}
			if ex != nil && p != abs && ex.Match(name) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			fi, ierr := d.Info()
			if ierr != nil {
				return nil
			}
			link := ""
			if fi.Mode()&fs.ModeSymlink != 0 {
				link, _ = os.Readlink(p)
			}
			if !fi.Mode().IsRegular() && !fi.IsDir() && link == "" {
				return nil
			}
			hdr, herr := tar.FileInfoHeader(fi, link)
			if herr != nil {
				return nil
			}
			hdr.Name = name
			if fi.IsDir() {
				hdr.Name += "/"
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if fi.Mode().IsRegular() {
				src, oerr := os.Open(p)
				if oerr != nil {
					return nil
				}
				n, cerr := io.Copy(tw, src)
				src.Close()
				if cerr != nil {
					return cerr
				}
				files++
				bytes += n
			}
			return nil
		})
		if e != nil {
			return files, bytes, e
		}
	}
	if err = tw.Close(); err != nil {
		return
	}
	if err = gz.Close(); err != nil {
		return
	}
	if enc != nil {
		if err = enc.Close(); err != nil {
			return
		}
	}
	if err = bw.Flush(); err != nil {
		return
	}
	if err = f.Sync(); err != nil {
		return
	}
	f.Close()
	err = os.Rename(tmp, out)
	return
}

// Extract unpacks an archive into dir.
func Extract(in, dir, password string) (int64, error) {
	f, err := os.Open(in)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	var r io.Reader = bufio.NewReaderSize(f, 1<<20)
	if strings.HasSuffix(in, ".snarc") {
		if r, err = newDecReader(r, password); err != nil {
			return 0, err
		}
	}
	gz, err := gzip.NewReader(r)
	if err != nil {
		return 0, err
	}
	tr := tar.NewReader(gz)
	base, _ := filepath.Abs(dir)
	var n int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		dst := filepath.Join(base, filepath.FromSlash(h.Name))
		rel, rerr := filepath.Rel(base, dst)
		if rerr != nil || strings.HasPrefix(rel, "..") {
			return n, fmt.Errorf("unsicherer Pfad im Archiv: %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(dst, 0o700)
		case tar.TypeSymlink:
			_ = os.MkdirAll(filepath.Dir(dst), 0o700)
			_ = os.Symlink(h.Linkname, dst)
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(dst), 0o700)
			out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode)&0o777)
			if err != nil {
				return n, err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return n, err
			}
			out.Close()
			_ = os.Chtimes(dst, h.ModTime, h.ModTime)
			n++
		}
	}
}
