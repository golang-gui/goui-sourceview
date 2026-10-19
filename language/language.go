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
	TOML  ID = "toml"
	YAML  ID = "yaml"
	Shell ID = "shell"
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
	KindVariable
)

var kindNames = [...]string{"plain", "property", "string", "escape", "number", "keyword", "delimiter", "tag", "attribute", "entity", "comment", "meta", "invalid", "variable"}

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
	{ID: TOML, Name: "TOML", Extensions: []string{".toml"}, MIMETypes: []string{"application/toml"}},
	{ID: YAML, Name: "YAML", Extensions: []string{".yaml", ".yml"}, MIMETypes: []string{"application/yaml"}},
	{ID: Shell, Name: "Shell", Extensions: []string{".sh", ".bash"}, MIMETypes: []string{"application/x-sh", "application/x-shellscript"}},
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
	switch filepath.Base(filename) {
	case ".bashrc", ".bash_profile", ".bash_login", ".bash_logout", ".profile":
		return Shell
	}
	sample = sample[:min(len(sample), 4096)]
	if shellShebang(sample) {
		return Shell
	}
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

// A shebang is recognized only at byte zero. No command is evaluated.
func shellShebang(sample []byte) bool {
	if !bytes.HasPrefix(sample, []byte("#!")) {
		return false
	}
	line, _, _ := bytes.Cut(sample[2:], []byte("\n"))
	fields := strings.Fields(string(line))
	if len(fields) == 0 {
		return false
	}
	interpreter := filepath.Base(fields[0])
	if interpreter == "env" {
		fields = fields[1:]
		for len(fields) > 0 {
			arg := fields[0]
			switch {
			case arg == "--":
				fields = fields[1:]
				goto interpreter
			case arg == "-S" || arg == "--split-string" || arg == "-i" || arg == "--ignore-environment":
				fields = fields[1:]
			case arg == "-u" || arg == "--unset" || arg == "-C" || arg == "--chdir":
				if len(fields) < 2 {
					return false
				}
				fields = fields[2:]
			case strings.HasPrefix(arg, "--unset=") || strings.HasPrefix(arg, "--chdir=") || !strings.HasPrefix(arg, "-") && strings.Contains(arg, "="):
				fields = fields[1:]
			case strings.HasPrefix(arg, "-"):
				return false
			default:
				goto interpreter
			}
		}
	interpreter:
		if len(fields) == 0 {
			return false
		}
		interpreter = filepath.Base(fields[0])
	}
	return interpreter == "sh" || interpreter == "bash"
}
