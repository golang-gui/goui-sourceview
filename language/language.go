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
	Plain      ID = ""
	JSON       ID = "json"
	JSONC      ID = "jsonc"
	XML        ID = "xml"
	HTML       ID = "html"
	TOML       ID = "toml"
	YAML       ID = "yaml"
	Shell      ID = "shell"
	HTTP       ID = "http"
	Makefile   ID = "makefile"
	CMake      ID = "cmake"
	Nginx      ID = "nginx"
	INI        ID = "ini"
	Dockerfile ID = "dockerfile"
	Markdown   ID = "markdown"
)

// Kind is an extensible style key. Lexical behavior uses Token.Class and Context.
// Custom keys may contain ASCII letters, digits, underscores, hyphens and dots.
type Kind string

const (
	KindPlain     Kind = ""
	KindProperty  Kind = "property"
	KindString    Kind = "string"
	KindEscape    Kind = "escape"
	KindNumber    Kind = "number"
	KindKeyword   Kind = "keyword"
	KindDelimiter Kind = "delimiter"
	KindTag       Kind = "tag"
	KindAttribute Kind = "attribute"
	KindEntity    Kind = "entity"
	KindComment   Kind = "comment"
	KindMeta      Kind = "meta"
	KindInvalid   Kind = "invalid"
	KindVariable  Kind = "variable"
)

func (k Kind) Valid() bool {
	if k == "plain" {
		return false
	}
	for _, c := range k {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return len(k) <= 128
}
func (k Kind) String() string {
	if k == KindPlain {
		return "plain"
	}
	return string(k)
}

// Info describes a language. Lookup returns independent metadata slices.
type Info struct {
	ID         ID
	Name       string
	Extensions []string
	MIMETypes  []string
}

var builtins = []Info{
	{ID: Plain, Name: "Plain text", Extensions: []string{".txt"}, MIMETypes: []string{"text/plain"}},
	{ID: JSON, Name: "JSON", Extensions: []string{".json"}, MIMETypes: []string{"application/json"}},
	{ID: JSONC, Name: "JSON with comments", Extensions: []string{".jsonc"}, MIMETypes: []string{"application/jsonc"}},
	{ID: XML, Name: "XML", Extensions: []string{".xml", ".xhtml", ".svg", ".xsd", ".xsl"}, MIMETypes: []string{"application/xml", "text/xml", "image/svg+xml"}},
	{ID: HTML, Name: "HTML", Extensions: []string{".html", ".htm"}, MIMETypes: []string{"text/html"}},
	{ID: TOML, Name: "TOML", Extensions: []string{".toml"}, MIMETypes: []string{"application/toml"}},
	{ID: YAML, Name: "YAML", Extensions: []string{".yaml", ".yml"}, MIMETypes: []string{"application/yaml"}},
	{ID: Shell, Name: "Shell", Extensions: []string{".sh", ".bash"}, MIMETypes: []string{"application/x-sh", "application/x-shellscript"}},
	{ID: HTTP, Name: "HTTP requests", Extensions: []string{".http", ".rest"}, MIMETypes: []string{"application/http"}},
	{ID: Makefile, Name: "Makefile", Extensions: []string{".mk", ".mak"}, MIMETypes: []string{"text/x-makefile"}},
	{ID: CMake, Name: "CMake", Extensions: []string{".cmake"}, MIMETypes: []string{"text/x-cmake"}},
	{ID: Nginx, Name: "Nginx", Extensions: []string{".nginx"}, MIMETypes: []string{"text/x-nginx-conf"}},
	{ID: INI, Name: "INI", Extensions: []string{".ini", ".cfg"}, MIMETypes: []string{"text/x-ini"}},
	{ID: Dockerfile, Name: "Dockerfile", Extensions: []string{".dockerfile"}, MIMETypes: []string{"text/x-dockerfile"}},
	{ID: Markdown, Name: "Markdown", Extensions: []string{".md", ".markdown"}, MIMETypes: []string{"text/markdown"}},
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
	base := filepath.Base(filename)
	switch {
	case base == "CMakeLists.txt":
		return CMake
	case base == "Makefile" || base == "makefile" || base == "GNUmakefile" || strings.HasPrefix(base, "Makefile."):
		return Makefile
	case base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile."):
		return Dockerfile
	case base == "nginx.conf" || strings.HasSuffix(base, ".nginx.conf"):
		return Nginx
	}
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
