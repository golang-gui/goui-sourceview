// Package language describes the languages and lexical styles supported by SourceView.
package language

import (
	"bytes"
	"path/filepath"
	"strings"
)

// ID identifies a language independently of its presentation.
type ID string

const (
	Plain ID = ""
	JSON  ID = "json"
	XML   ID = "xml"
	HTML  ID = "html"
)

// Kind is a lexical style category, not a validation or semantic diagnosis.
type Kind uint8

const (
	KindPlain Kind = iota
	KindProperty
	KindString
	KindEscape
	KindNumber
	KindKeyword
	KindDelimiter
	KindTag
	KindAttribute
	KindEntity
	KindComment
	KindMeta
	KindInvalid
)

var kindNames = [...]string{"plain", "property", "string", "escape", "number", "keyword", "delimiter", "tag", "attribute", "entity", "comment", "meta", "invalid"}

// Valid reports whether k is a defined lexical kind.
func (k Kind) Valid() bool { return int(k) < len(kindNames) }

func (k Kind) String() string {
	if k.Valid() {
		return kindNames[k]
	}
	return "unknown"
}

// Info describes an immutable built-in language. Lookup returns independent slices.
type Info struct {
	ID         ID
	Name       string
	Extensions []string
	MIMETypes  []string
}

var builtins = []Info{
	{ID: Plain, Name: "Plain text", Extensions: []string{".txt"}, MIMETypes: []string{"text/plain"}},
	{ID: JSON, Name: "JSON", Extensions: []string{".json"}, MIMETypes: []string{"application/json"}},
	{ID: XML, Name: "XML", Extensions: []string{".xml", ".xhtml", ".svg", ".xsd", ".xsl"}, MIMETypes: []string{"application/xml", "text/xml", "image/svg+xml"}},
	{ID: HTML, Name: "HTML", Extensions: []string{".html", ".htm"}, MIMETypes: []string{"text/html"}},
}

// Lookup returns a detached description of a supported language.
func Lookup(id ID) (Info, bool) {
	for _, info := range builtins {
		if info.ID == id {
			info.Extensions = append([]string(nil), info.Extensions...)
			info.MIMETypes = append([]string(nil), info.MIMETypes...)
			return info, true
		}
	}
	return Info{}, false
}

// Guess examines an extension and at most 4 KiB of caller-supplied content.
// It performs no I/O. Unknown or ambiguous input is plain text.
func Guess(filename string, sample []byte) ID {
	ext := strings.ToLower(filepath.Ext(filename))
	for _, info := range builtins {
		for _, candidate := range info.Extensions {
			if ext == candidate {
				return info.ID
			}
		}
	}
	sample = sample[:min(len(sample), 4096)]
	sample = bytes.TrimSpace(bytes.TrimPrefix(sample, []byte{0xef, 0xbb, 0xbf}))
	lower := bytes.ToLower(sample)
	if bytes.HasPrefix(sample, []byte("<?xml")) {
		return XML
	}
	if bytes.HasPrefix(lower, []byte("<!doctype html")) || bytes.HasPrefix(lower, []byte("<html")) {
		return HTML
	}
	if len(sample) > 0 && (sample[0] == '{' || sample[0] == '[') {
		return JSON
	}
	return Plain
}
