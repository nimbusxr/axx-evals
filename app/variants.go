package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"sort"
	"strings"
)

// The correct variants (internal/mutant.Variants) that change how responses
// are written. The others live where they apply: the importer, the tracking
// projector and the event publisher.

// reorderJSON is the json-properties-reordered variant: every JSON response
// body (application/json and any +json type) is written again with each
// object's properties in reverse alphabetical order. It means the same; only
// tests that compare JSON as text notice.
func reorderJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		body := rec.body.Bytes()
		if isJSONType(rec.header.Get("Content-Type")) && len(bytes.TrimSpace(body)) > 0 {
			if b, err := reorderedJSON(body); err == nil {
				body = b
			}
		}
		for k, v := range rec.header {
			w.Header()[k] = v
		}
		w.Header().Del("Content-Length")
		w.WriteHeader(rec.status)
		_, _ = w.Write(body)
	})
}

type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header         { return b.header }
func (b *bufferedResponse) WriteHeader(status int)      { b.status = status }
func (b *bufferedResponse) Write(p []byte) (int, error) { return b.body.Write(p) }

func isJSONType(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	return err == nil && (mt == "application/json" || strings.HasSuffix(mt, "+json"))
}

// reorderedJSON writes doc again with every object's properties in reverse
// alphabetical order, keeping numbers exactly as they were.
func reorderedJSON(doc []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := writeReordered(&out, v); err != nil {
		return nil, err
	}
	if bytes.HasSuffix(doc, []byte("\n")) {
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func writeReordered(out *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(keys)))
		out.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			out.Write(kb)
			out.WriteByte(':')
			if err := writeReordered(out, t[k]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	case []any:
		out.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeReordered(out, e); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case json.Number:
		out.WriteString(t.String())
	case string, bool, nil:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		out.Write(b)
	default:
		return fmt.Errorf("unexpected JSON value %T", v)
	}
	return nil
}
