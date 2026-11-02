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
	Plain         ID = ""
	JSON          ID = "json"
	JSONC         ID = "jsonc"
	JSONLines     ID = "jsonlines"
	XML           ID = "xml"
	HTML          ID = "html"
	TOML          ID = "toml"
	YAML          ID = "yaml"
	Shell         ID = "shell"
	HTTP          ID = "http"
	Makefile      ID = "makefile"
	CMake         ID = "cmake"
	Nginx         ID = "nginx"
	INI           ID = "ini"
	Dockerfile    ID = "dockerfile"
	SQL           ID = "sql"
	Protobuf      ID = "protobuf"
	Go            ID = "go"
	Python        ID = "python"
	JavaScript    ID = "javascript"
	TypeScript    ID = "typescript"
	CSS           ID = "css"
	C             ID = "c"
	Cpp           ID = "cpp"
	Java          ID = "java"
	Rust          ID = "rust"
	CSharp        ID = "csharp"
	Lua           ID = "lua"
	PowerShell    ID = "powershell"
	Ruby          ID = "ruby"
	PHP           ID = "php"
	JSX           ID = "jsx"
	TSX           ID = "tsx"
	Kotlin        ID = "kotlin"
	Swift         ID = "swift"
	Dart          ID = "dart"
	R             ID = "r"
	Zig           ID = "zig"
	ObjectiveC    ID = "objective-c"
	ObjectiveCpp  ID = "objective-cpp"
	Scala         ID = "scala"
	Perl          ID = "perl"
	Diff          ID = "diff"
	Batch         ID = "bat"
	HCL           ID = "hcl"
	GraphQL       ID = "graphql"
	SCSS          ID = "scss"
	Less          ID = "less"
	GoMod         ID = "gomod"
	GoWork        ID = "gowork"
	GoSum         ID = "gosum"
	Dotenv        ID = "dotenv"
	Properties    ID = "properties"
	GitIgnore     ID = "gitignore"
	GitAttributes ID = "gitattributes"
	Starlark      ID = "starlark"
	Groovy        ID = "groovy"
	GoTemplate    ID = "gotemplate"
	Jinja2        ID = "jinja2"
	Systemd       ID = "systemd"
	DesktopEntry  ID = "desktop"
	Nix           ID = "nix"
	JQ            ID = "jq"

	Markdown ID = "markdown"
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
	{ID: JSONLines, Name: "JSON Lines", Extensions: []string{".jsonl", ".ndjson"}, MIMETypes: []string{"application/jsonl", "application/x-ndjson"}},
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
	{ID: SQL, Name: "SQL", Extensions: []string{".sql"}, MIMETypes: []string{"application/sql"}},
	{ID: Protobuf, Name: "Protocol Buffers", Extensions: []string{".proto"}, MIMETypes: []string{"text/x-protobuf"}},
	{ID: Go, Name: "Go", Extensions: []string{".go"}, MIMETypes: []string{"text/x-go"}},
	{ID: Python, Name: "Python", Extensions: []string{".py", ".pyi", ".pyw"}, MIMETypes: []string{"text/x-python"}},
	{ID: JavaScript, Name: "JavaScript", Extensions: []string{".js", ".mjs", ".cjs"}, MIMETypes: []string{"text/javascript", "application/javascript"}},
	{ID: TypeScript, Name: "TypeScript", Extensions: []string{".ts", ".mts", ".cts"}, MIMETypes: []string{"text/typescript", "application/typescript"}},
	{ID: CSS, Name: "CSS", Extensions: []string{".css"}, MIMETypes: []string{"text/css"}},
	{ID: C, Name: "C", Extensions: []string{".c", ".i"}, MIMETypes: []string{"text/x-csrc"}},
	{ID: Cpp, Name: "C++", Extensions: []string{".cpp", ".cc", ".cxx", ".c++", ".h", ".hpp", ".hh", ".hxx", ".h++", ".ipp", ".inl", ".ixx", ".cppm", ".ii"}, MIMETypes: []string{"text/x-c++src", "text/x-c++hdr", "text/x-chdr"}},
	{ID: Java, Name: "Java", Extensions: []string{".java"}, MIMETypes: []string{"text/x-java"}},
	{ID: Rust, Name: "Rust", Extensions: []string{".rs"}, MIMETypes: []string{"text/x-rust"}},
	{ID: CSharp, Name: "C#", Extensions: []string{".cs", ".csx"}, MIMETypes: []string{"text/x-csharp"}},
	{ID: Lua, Name: "Lua", Extensions: []string{".lua"}, MIMETypes: []string{"text/x-lua"}},
	{ID: PowerShell, Name: "PowerShell", Extensions: []string{".ps1", ".psm1", ".psd1"}, MIMETypes: []string{"application/x-powershell"}},
	{ID: Ruby, Name: "Ruby", Extensions: []string{".rb", ".rbw", ".rake", ".gemspec", ".ru"}, MIMETypes: []string{"text/x-ruby"}},
	{ID: PHP, Name: "PHP", Extensions: []string{".php", ".phtml"}, MIMETypes: []string{"application/x-httpd-php"}},
	{ID: JSX, Name: "JSX", Extensions: []string{".jsx"}, MIMETypes: []string{"text/jsx", "text/javascript-jsx"}},
	{ID: TSX, Name: "TSX", Extensions: []string{".tsx"}, MIMETypes: []string{"text/tsx", "text/typescript-jsx"}},
	{ID: Kotlin, Name: "Kotlin", Extensions: []string{".kt", ".kts"}, MIMETypes: []string{"text/x-kotlin"}},
	{ID: Swift, Name: "Swift", Extensions: []string{".swift", ".swiftinterface"}, MIMETypes: []string{"text/x-swift"}},
	{ID: Dart, Name: "Dart", Extensions: []string{".dart"}, MIMETypes: []string{"text/x-dart"}},
	{ID: R, Name: "R", Extensions: []string{".r", ".rscript"}, MIMETypes: []string{"text/x-r-source"}},
	{ID: Zig, Name: "Zig", Extensions: []string{".zig"}, MIMETypes: []string{"text/x-zig"}},
	{ID: ObjectiveC, Name: "Objective-C", Extensions: []string{".m"}, MIMETypes: []string{"text/x-objcsrc"}},
	{ID: ObjectiveCpp, Name: "Objective-C++", Extensions: []string{".mm"}, MIMETypes: []string{"text/x-objc++src"}},
	{ID: Scala, Name: "Scala", Extensions: []string{".scala", ".sbt"}, MIMETypes: []string{"text/x-scala"}},
	{ID: Perl, Name: "Perl", Extensions: []string{".pl", ".pm"}, MIMETypes: []string{"text/x-perl"}},
	{ID: Diff, Name: "Diff", Extensions: []string{".diff", ".patch"}, MIMETypes: []string{"text/x-diff", "text/x-patch"}},
	{ID: Batch, Name: "Windows Batch", Extensions: []string{".bat", ".cmd"}, MIMETypes: []string{"application/x-bat"}},
	{ID: HCL, Name: "HCL / Terraform", Extensions: []string{".hcl", ".tf", ".tfvars"}, MIMETypes: []string{"text/x-hcl"}},
	{ID: GraphQL, Name: "GraphQL", Extensions: []string{".graphql", ".gql", ".graphqls"}, MIMETypes: []string{"application/graphql", "text/x-graphql"}},
	{ID: SCSS, Name: "SCSS", Extensions: []string{".scss"}, MIMETypes: []string{"text/x-scss"}},
	{ID: Less, Name: "Less", Extensions: []string{".less"}, MIMETypes: []string{"text/x-less"}},
	{ID: GoMod, Name: "Go Module", MIMETypes: []string{"text/x-go-mod"}},
	{ID: GoWork, Name: "Go Workspace", MIMETypes: []string{"text/x-go-work"}},
	{ID: GoSum, Name: "Go Checksums", MIMETypes: []string{"text/x-go-sum"}},
	{ID: Dotenv, Name: "Dotenv", Extensions: []string{".env"}, MIMETypes: []string{"text/x-dotenv"}},
	{ID: Properties, Name: "Java Properties", Extensions: []string{".properties"}, MIMETypes: []string{"text/x-java-properties"}},
	{ID: GitIgnore, Name: "Git Ignore", Extensions: []string{".gitignore"}, MIMETypes: []string{"text/x-gitignore"}},
	{ID: GitAttributes, Name: "Git Attributes", Extensions: []string{".gitattributes"}, MIMETypes: []string{"text/x-gitattributes"}},
	{ID: Starlark, Name: "Starlark", Extensions: []string{".bzl", ".star", ".bazel"}, MIMETypes: []string{"text/x-starlark"}},
	{ID: Groovy, Name: "Groovy", Extensions: []string{".groovy", ".gvy", ".gy", ".gsh", ".gradle"}, MIMETypes: []string{"text/x-groovy"}},
	{ID: GoTemplate, Name: "Go Template", Extensions: []string{".gotmpl", ".gohtml"}},
	{ID: Jinja2, Name: "Jinja2", Extensions: []string{".j2", ".jinja", ".jinja2"}},
	{ID: Systemd, Name: "systemd unit", Extensions: []string{".service", ".socket", ".device", ".mount", ".automount", ".swap", ".target", ".path", ".timer", ".slice", ".scope"}},
	{ID: DesktopEntry, Name: "Desktop Entry", Extensions: []string{".desktop", ".directory"}, MIMETypes: []string{"application/x-desktop"}},
	{ID: Nix, Name: "Nix", Extensions: []string{".nix"}},
	{ID: JQ, Name: "jq", Extensions: []string{".jq"}},

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

// Guess examines a filename and at most 4 KiB of caller-supplied content.
// It performs no I/O. Unknown or ambiguous input is plain text.
func Guess(filename string, sample []byte) ID {
	base := filepath.Base(filename)
	switch {
	case base == "Jenkinsfile":
		return Groovy
	case base == "BUILD" || base == "WORKSPACE":
		return Starlark
	case strings.HasPrefix(base, ".env."):
		return Dotenv
	case base == "go.mod":
		return GoMod
	case base == "go.work":
		return GoWork
	case base == "go.sum" || base == "go.work.sum":
		return GoSum
	case base == ".Rprofile":
		return R
	case base == "CMakeLists.txt":
		return CMake
	case base == "Makefile" || base == "makefile" || base == "GNUmakefile" || strings.HasPrefix(base, "Makefile."):
		return Makefile
	case base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile."):
		return Dockerfile
	case base == "nginx.conf" || strings.HasSuffix(base, ".nginx.conf"):
		return Nginx
	case base == "Gemfile" || base == "Rakefile":
		return Ruby
	}
	// Uppercase .C is a conventional C++ suffix, even on case-sensitive hosts.
	if filepath.Ext(filename) == ".C" {
		return Cpp
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".m" {
		// MATLAB shares this suffix. Require a leading Objective-C construct.
		for _, line := range strings.Split(string(sample[:min(len(sample), 4096)]), "\n") {
			line = strings.TrimSpace(line)
			for _, marker := range []string{"#import", "@interface", "@implementation", "@protocol", "@class", "@import", "@autoreleasepool"} {
				if strings.HasPrefix(line, marker) && (len(line) == len(marker) || strings.ContainsRune(" \t({\"<", rune(line[len(marker)]))) {
					return ObjectiveC
				}
			}
		}
		return Plain
	}
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
	interpreter := shebangInterpreter(sample)
	switch interpreter {
	case "jq":
		return JQ

	}
	if interpreter == "groovy" {
		return Groovy
	}
	perlVersion := strings.TrimPrefix(interpreter, "perl5.")
	if interpreter == "perl" || perlVersion != interpreter && len(perlVersion) > 0 && digit(perlVersion[0]) && strings.Trim(perlVersion, "0123456789.") == "" {
		return Perl
	}
	if interpreter == "scala" || interpreter == "scala-cli" {
		return Scala
	}
	if interpreter == "Rscript" {
		return R
	}
	if interpreter == "dart" {
		return Dart
	}
	if interpreter == "swift" {
		return Swift
	}
	if interpreter == "kotlin" {
		return Kotlin
	}
	if interpreter == "ruby" {
		return Ruby
	}
	if interpreter == "pwsh" || interpreter == "powershell" {
		return PowerShell
	}
	pythonVersion := strings.TrimPrefix(interpreter, "python")
	if interpreter == "python" || pythonVersion != interpreter && len(pythonVersion) > 0 && digit(pythonVersion[0]) && strings.Trim(pythonVersion, "0123456789.") == "" {
		return Python
	}
	for _, name := range []string{"lua", "luajit"} {
		version := strings.TrimPrefix(interpreter, name)
		if interpreter == name || version != interpreter && len(version) > 0 && digit(version[0]) && strings.Trim(version, "0123456789.") == "" {
			return Lua
		}
	}
	sample = bytes.TrimSpace(bytes.TrimPrefix(sample, []byte{0xef, 0xbb, 0xbf}))
	lower := bytes.ToLower(sample)
	if phpOpening(string(sample)) > 0 {
		return PHP
	}
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
	interpreter := shebangInterpreter(sample)
	return interpreter == "sh" || interpreter == "bash"
}

func shebangInterpreter(sample []byte) string {
	if !bytes.HasPrefix(sample, []byte("#!")) {
		return ""
	}
	line, _, _ := bytes.Cut(sample[2:], []byte("\n"))
	fields := strings.Fields(string(line))
	if len(fields) == 0 {
		return ""
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
					return ""
				}
				fields = fields[2:]
			case strings.HasPrefix(arg, "--unset=") || strings.HasPrefix(arg, "--chdir=") || !strings.HasPrefix(arg, "-") && strings.Contains(arg, "="):
				fields = fields[1:]
			case strings.HasPrefix(arg, "-"):
				return ""
			default:
				goto interpreter
			}
		}
	interpreter:
		if len(fields) == 0 {
			return ""
		}
		interpreter = filepath.Base(fields[0])
	}
	return interpreter
}
