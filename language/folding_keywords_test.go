package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBuiltinKeywordFolding(t *testing.T) {
	for _, id := range []ID{Lua, Ruby, Shell, CMake, SQL} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			if l.Folding() == nil {
				t.Fatal("builtin has no keyword folding provider")
			}
		})
	}
}

func TestKeywordFoldingStructures(t *testing.T) {
	for _, tt := range []struct {
		name string
		id   ID
		text string
		want []FoldRange
	}{
		{"lua loops", Lua, "local function f()\n for i = 1, 3 do\n  work()\n end\nend", []FoldRange{{0, 4}, {1, 3}}},
		{"lua branches", Lua, "if x then\n do\n  work()\n end\nelseif y then\n other()\nend", []FoldRange{{0, 6}, {1, 3}}},
		{"lua repeat", Lua, "repeat\n work()\nuntil ready()", []FoldRange{{0, 2}}},
		{"lua repeat continued", Lua, "repeat\n work()\nuntil a and\n (b or c)", []FoldRange{{0, 3}}},
		{"lua empty until", Lua, "repeat\n work()\nuntil", nil},
		{"lua anonymous", Lua, "local f = function()\n return {\n  1, 2\n }\nend", []FoldRange{{0, 4}, {1, 3}}},
		{"lua incomplete", Lua, "for i = 1, 2\n work()\nend", nil},
		{"ruby blocks and modifier", Ruby, "class A\n def f(x = 1)\n  work if ready\n end\nend", []FoldRange{{0, 4}, {1, 3}}},
		{"ruby assignment if", Ruby, "x = if ready\n one\nelse\n two\nend", []FoldRange{{0, 4}}},
		{"ruby loop do", Ruby, "while ready do\n work\nend", []FoldRange{{0, 2}}},
		{"ruby nested do", Ruby, "while ready\n values.each do |x|\n  work(x)\n end\nend", []FoldRange{{0, 4}, {1, 3}}},
		{"ruby endless definitions", Ruby, "class A\n def f(x) = x + 1\n def g = 2\nend", []FoldRange{{0, 3}}},
		{"ruby endless compact", Ruby, "class A\n def f(x)=x+1\nend", []FoldRange{{0, 2}}},
		{"ruby bare default argument", Ruby, "def f x = 1\n work\nend", []FoldRange{{0, 2}}},
		{"ruby setter", Ruby, "def name=(x)\n @name = x\nend", []FoldRange{{0, 2}}},
		{"ruby keyword method", Ruby, "def if(x)\n x\nend", []FoldRange{{0, 2}}},
		{"ruby conditional default", Ruby, "def f(x = if ready\n one\nelse\n two\nend)\n work\nend", []FoldRange{{0, 6}}},
		{"ruby postfix only", Ruby, "work if ready\nwork unless ready\nwork while ready\nwork until ready", nil},
		{"ruby members", Ruby, "class A\n obj.if(x)\n obj.end\nend", []FoldRange{{0, 3}}},
		{"cmake nested commands", CMake, "IF(READY)\n function(f)\n  message(ok)\n endfunction()\nelse()\n message(other)\nENDIF()", []FoldRange{{0, 6}, {1, 3}}},
		{"cmake multiline args", CMake, "set(VALUES\n one\n two\n)", []FoldRange{{0, 3}}},
		{"cmake fake commands", CMake, "message(if endif function endfunction)\nset(x \"if()\nendif()\")", []FoldRange{{1, 2}}},
		{"cmake mismatch", CMake, "if(x)\n work()\nendfunction()", nil},
		{"cmake unclosed header", CMake, "if(\n x\nendif()", nil},
		{"sql nested case", SQL, "SELECT CASE WHEN x THEN\n CASE y\n  WHEN 1 THEN 'one'\n  ELSE 'other'\n END\nELSE 'none'\nEND;", []FoldRange{{0, 6}, {1, 4}}},
		{"sql alias closing line", SQL, "SELECT CASE\n WHEN x THEN 1\n END AS value;", []FoldRange{{0, 1}}},
		{"sql transaction", SQL, "BEGIN;\nUPDATE t SET x=1;\nCOMMIT;", nil},
		{"sql unsupported procedural", SQL, "BEGIN\n IF x THEN\n  work();\n END IF;\nEND;", nil},
		{"sql qualified end", SQL, "CASE x\n WHEN 1 THEN 2\nEND CASE;", []FoldRange{{0, 1}}},
		{"sql incomplete", SQL, "SELECT CASE\n WHEN x THEN 1", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := Builtin(tt.id)
			got, err := l.Folding()(context.Background(), tt.text)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("folds = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestKeywordFoldingLiterals(t *testing.T) {
	for _, tt := range []struct {
		name string
		id   ID
		text string
		want []FoldRange
	}{
		{"lua long string", Lua, "local x = [==[\n end if ]=] }\n]==]", []FoldRange{{0, 2}}},
		{"lua long comment", Lua, "--[=[\nfunction f() end\n]=]", []FoldRange{{0, 2}}},
		{"lua escaped whitespace", Lua, "local x = \"a\\z\n  \n end\"", []FoldRange{{0, 2}}},
		{"lua adjacent literals", Lua, "x = [[\n]]; y = [[\n]]", []FoldRange{{1, 2}}},
		{"ruby percent literal", Ruby, "x = %q{\n end if {nested}\n}", []FoldRange{{0, 2}}},
		{"ruby interpolation", Ruby, "x = \"start #{\n if ready\n  %q{inner\n  end }\n end\n} finish\"", []FoldRange{{0, 5}, {1, 4}, {2, 3}}},
		{"ruby heredoc queue", Ruby, "puts <<A, <<'B'\n end\nA\n if\nB", []FoldRange{{0, 4}}},
		{"ruby nested heredoc", Ruby, "puts <<OUTER\n #{call(<<INNER)}\n end\nINNER\n more\nOUTER", []FoldRange{{0, 5}, {1, 3}}},
		{"ruby heredoc recovery", Ruby, "x = \"outer #{call(<<EOF)}\n #{ \"broken\nEOF\nrest\"", []FoldRange{{0, 3}}},
		{"ruby heredoc block recovery", Ruby, "class A\n x = \"outer #{call(<<EOF)}\n #{ if broken\nEOF\nrest\"\nend", []FoldRange{{0, 5}, {1, 4}}},
		{"ruby block comment", Ruby, "=begin\n class A end\n=end", []FoldRange{{0, 2}}},
		{"ruby data", Ruby, "__END__\nclass A\nend", nil},
		{"ruby adjacent strings", Ruby, "x = '\n'; y = '\n'", []FoldRange{{1, 2}}},
		{"cmake bracket literal", CMake, "set(x [==[\nendif() # function()\n]==])", []FoldRange{{0, 2}}},
		{"cmake bracket comment", CMake, "#[=[\nif(x)\n]=]", []FoldRange{{0, 2}}},
		{"cmake adjacent strings", CMake, "set(x \"\n\" \"\n\")", []FoldRange{{0, 2}}},
		{"sql dollar quote", SQL, "SELECT $body$\nCASE END BEGIN\n$body$;", []FoldRange{{0, 2}}},
		{"sql dollar close cannot cross LF", SQL, "SELECT $tag$\nbody $ta\ng$\nCASE END BEGIN\n$tag$;", []FoldRange{{0, 4}}},
		{"sql nested comments", SQL, "/* outer\n /* inner\n  CASE END\n */\n*/", []FoldRange{{0, 4}, {1, 3}}},
		{"sql repeated quote", SQL, "SELECT 'start\n it''s END CASE\nend';", []FoldRange{{0, 2}}},
		{"sql quoted identifiers", SQL, "SELECT [CASE], \"END\", `BEGIN`\nFROM data", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := Builtin(tt.id)
			got, err := l.Folding()(context.Background(), tt.text)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("folds = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestShellKeywordFolding(t *testing.T) {
	l, _ := Builtin(Shell)
	fold := l.Folding()
	for _, tt := range []struct {
		name, text string
		want       []FoldRange
	}{
		{"if and arguments", "if test -f x; then\n echo fi done if\nfi", []FoldRange{{0, 2}}},
		{"nested loops", "for x in a b; do\n if test x; then\n  echo x\n fi\ndone", []FoldRange{{0, 4}, {1, 3}}},
		{"branches", "if true; then\n echo a\nelif false; then\n echo b\nelse\n echo c\nfi", []FoldRange{{0, 6}}},
		{"case patterns", "demo() {\n case x in\n  if|done) echo fi ;;\n  *) echo done ;;\n esac\n}", []FoldRange{{0, 5}, {1, 4}}},
		{"pattern nesting", "case x in\n @(a|b)) echo done ;;&\n (c) echo if ;&\n *) echo fi ;;\nesac", []FoldRange{{0, 4}}},
		{"command substitution", "x=$(\n if true; then\n  echo x\n fi\n)", []FoldRange{{0, 4}, {1, 3}}},
		{"quoted command substitution", "x=\"before $(\n if true; then\n  echo x\n fi\n) after\"", []FoldRange{{0, 4}, {1, 3}}},
		{"backtick", "x=`\n if true; then\n  echo x\n fi\n`", []FoldRange{{0, 4}, {1, 3}}},
		{"parameters and arithmetic", "echo ${x:-if} $((if + 1))\necho fi done case", nil},
		{"quoted keywords", "echo 'if\n fi done'\n echo \\\n if", []FoldRange{{0, 1}}},
		{"heredoc fake syntax", "cat <<'EOF'\nif false; then\n echo done\nEOF", []FoldRange{{0, 3}}},
		{"multiple heredocs", "cat <<A <<-B\nif\nA\nfi\n\tB", []FoldRange{{0, 4}}},
		{"nested heredoc", "x=$(cat <<EOF\nif done\nEOF\n)", []FoldRange{{0, 3}}},
		{"literal trailing command", "if true; then\n echo x\nfi; echo other", []FoldRange{{0, 1}}},
		{"missing then", "if true\n echo x\nfi", nil},
		{"wrong closer", "if true; then\n echo x\ndone", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fold(context.Background(), tt.text)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("folds = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestKeywordFoldingWorkers(t *testing.T) {
	for _, tt := range []struct {
		id          ID
		open, close string
	}{
		{Lua, "x = [=[", "]=]"},
		{Ruby, "x = <<'EOF'", "EOF"},
		{Shell, "cat <<'EOF'", "EOF"},
		{CMake, "#[=[", "]=]"},
		{SQL, "SELECT $q$", "$q$;"},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			fold := l.Folding()
			var workers sync.WaitGroup
			for i := 0; i < 8; i++ {
				workers.Add(1)
				go func(i int) {
					defer workers.Done()
					text := strings.Repeat("\n", i) + tt.open + "\n" + strings.Repeat("x", 9000) + "\n" + tt.close
					got, err := fold(context.Background(), text)
					if err != nil || !reflect.DeepEqual(got, []FoldRange{{i, i + 2}}) {
						t.Errorf("worker %d: %v, %v", i, got, err)
					}
				}(i)
			}
			workers.Wait()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			scanner := cancelCodeScanner{l.Tokenizer(), cancel}
			got, err := keywordFolding(tt.id, scanner)(ctx, "\n"+strings.Repeat("x ", 16000))
			if err != context.Canceled || got != nil {
				t.Fatalf("mid-scan cancellation = %v, %v", got, err)
			}
		})
	}
}

func TestKeywordFoldingFragmentEdges(t *testing.T) {
	for _, id := range []ID{Lua, Ruby, Shell, CMake, SQL} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			label := "TEXT" + strings.Repeat("x", 5000)
			for at := 4090; at <= 4097; at++ {
				pad := strings.Repeat(" ", at)
				var open, close string
				switch id {
				case Lua:
					open, close = "x = [====[", pad+"]====]"
				case Ruby:
					open, close = "puts <<~'"+label+"'", pad+label
				case Shell:
					open, close = "cat <<-'"+label+"'", strings.Repeat("\t", at)+label
				case CMake:
					open, close = "#[====[", pad+"]====]"
				case SQL:
					open, close = "SELECT $"+label+"$", pad+"$"+label+"$;"
				}
				text := pad + open + "\n if end fi CASE endif() }\n" + close
				got, err := l.Folding()(context.Background(), text)
				if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 2}}) {
					t.Fatalf("offset %d: %v, %v", at, got, err)
				}
			}
		})
	}
}

func TestSQLDollarDelimiterLineBoundary(t *testing.T) {
	l, _ := Builtin(SQL)
	scanner := l.Tokenizer()
	state := scanner.InitialState()
	for row, line := range []string{"SELECT $tag$", "body $ta", "g$", "CASE END", "$tag$;"} {
		r, err := scanner.Scan(Input{Text: line, Limit: len(line), LineEnd: true}, state)
		if err != nil {
			t.Fatal(err)
		}
		state = r.State
		if row >= 1 && row <= 3 && !state.Context().String {
			t.Fatalf("row %d left the dollar literal", row)
		}
		if row == 3 && (len(r.Tokens) != 1 || r.Tokens[0].Class != ClassString || r.Tokens[0].Start != 0 || r.Tokens[0].End != len(line)) {
			t.Fatalf("fake closing delimiter exposed SQL keywords: %v", r.Tokens)
		}
	}
	if state.Context().String {
		t.Fatal("real closing delimiter did not end the literal")
	}
}
