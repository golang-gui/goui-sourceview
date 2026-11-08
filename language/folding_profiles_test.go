package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBuiltinProfileFolding(t *testing.T) {
	for _, id := range []ID{ObjectiveC, ObjectiveCpp, SCSS, Less, Perl} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			if l.Folding() == nil {
				t.Fatal("builtin has no profile folding provider")
			}
		})
	}
}

func TestProfileFoldingRanges(t *testing.T) {
	for _, tt := range []struct {
		name string
		id   ID
		text string
		want []FoldRange
	}{
		{"objc declarations", ObjectiveC, "@interface Demo : NSObject\n@property id value;\n@end\n@implementation Demo\n- (void)run {\n [self work:@\"} @end\"];\n}\n@end", []FoldRange{{0, 2}, {3, 7}, {4, 6}}},
		{"objc categories", ObjectiveC, "@interface Demo (Extras)\n- (void)run;\n@end\n@implementation Demo (Extras)\n- (void)run {}\n@end", []FoldRange{{0, 2}, {3, 5}}},
		{"objc extension", ObjectiveC, "@interface Demo () {\n id _value;\n}\n@end", []FoldRange{{0, 3}}},
		{"objc protocol", ObjectiveC, "@protocol Runnable <NSObject>\n@required\n- (void)run;\n@end", []FoldRange{{0, 3}}},
		{"objc split protocol", ObjectiveC, "@protocol\nRunnable\n- (void)run;\n@end", []FoldRange{{0, 3}}},
		{"objc forwards and expression", ObjectiveC, "@class Demo;\n@protocol One, Two;\n@protocol Three\n;\nid p = @protocol(One);\n@implementation Demo\n- (void)run {}\n@end", []FoldRange{{5, 7}}},
		{"objc expression preserves declaration", ObjectiveC, "@implementation Demo\nstatic Protocol *p = @protocol(Runnable);\n@end", []FoldRange{{0, 2}}},
		{"objc forward preserves declaration", ObjectiveC, "@implementation Demo\n@protocol Runnable;\n@end", []FoldRange{{0, 2}}},
		{"objc expression in method", ObjectiveC, "@implementation Demo\n- (void)run {\n id p = @protocol(Runnable);\n}\n@end", []FoldRange{{0, 4}, {1, 3}}},
		{"objc false directives", ObjectiveC, "/* @interface Fake\n@end */\n@implementation Demo\nid x = @\"@end\";\n@end", []FoldRange{{0, 1}, {2, 4}}},
		{"objc missing name", ObjectiveC, "@interface\n@end", nil},
		{"objc incomplete", ObjectiveC, "@interface A\n@property id value;", nil},
		{"objc trailing source", ObjectiveC, "@interface A\n@property id value;\n@end int x;", []FoldRange{{0, 1}}},
		{"objc macro", ObjectiveC, "@implementation A\n#define CLOSE @end } \\\n @interface Fake\n- (void)run {\n work();\n}\n@end", []FoldRange{{0, 6}, {3, 5}}},
		{"objcpp raw literal", ObjectiveCpp, "@implementation A\n- (void)run {\n auto text = R\"tag(\n @end } /*\n)tag\";\n}\n@end", []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
		{"objc continued literal", ObjectiveC, "id text = @\"first\\\n @end }\\\n last\";", []FoldRange{{0, 2}}},
		{"objc adjacent comments", ObjectiveC, "/* first\n*/ id x; /* second\n*/", []FoldRange{{1, 2}}},
		{"scss nested rules", SCSS, "@mixin demo($name) {\n .#{$name} {\n  color: red;\n }\n}", []FoldRange{{0, 4}, {1, 3}}},
		{"scss interpolation literal", SCSS, ".demo {\n content: \"start #{\n  inspect((\n   a: \"}\",\n  ))\n } end\";\n}", []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
		{"scss comment interpolation", SCSS, "/* outer #{\n inspect(\"inner\")\n} tail\n*/", []FoldRange{{0, 3}}},
		{"scss url interpolation", SCSS, ".demo {\n background: url(\"https://example.test/#{\n  inspect(\"}\")\n }\");\n}", []FoldRange{{0, 4}, {1, 3}}},
		{"scss escaped interpolation", SCSS, ".demo {\n content: \"\\#{ fake }\";\n // }\n}", []FoldRange{{0, 3}}},
		{"scss adjacent comments", SCSS, "/* first\n*/ x; /* second\n*/", []FoldRange{{1, 2}}},
		{"scss incomplete interpolation", SCSS, ".demo {\n content: \"#{\n  inspect(1)", nil},
		{"scss nested comment followed by code", SCSS, "/* outer #{\n /* inner\n */ inspect(1)\n} tail\n*/", []FoldRange{{0, 4}}},
		{"scss nested interpolation comment", SCSS, "/* outer #{\n /* inner\n */\n inspect(1)\n} tail\n*/", []FoldRange{{0, 5}, {1, 2}}},
		{"less detached rules", Less, "@rules: {\n .demo {\n  color: red;\n }\n};", []FoldRange{{0, 4}, {1, 3}}},
		{"less variables", Less, ".@{name} {\n @{property}: \"@{value} }\";\n background: url(@{root}/icon.svg);\n}", []FoldRange{{0, 3}}},
		{"less opaque javascript", Less, ".demo {\n @value: `function() {\n  return '}';\n }`;\n}", []FoldRange{{0, 4}, {1, 3}}},
		{"less string continuation", Less, ".demo {\n content: \"first\\\n last\";\n}", []FoldRange{{0, 3}, {1, 2}}},
		{"less url comment", Less, ".demo {\n background: url(https://example.test/icon.svg);\n /* }\n */\n}", []FoldRange{{0, 4}, {2, 3}}},
		{"perl blocks", Perl, "sub demo {\n if ($ready) {\n  say q{ };\n }\n}", []FoldRange{{0, 4}, {1, 3}}},
		{"perl quote nesting", Perl, "my $x = q{first\n {nested} [\nlast};", []FoldRange{{0, 2}}},
		{"perl multiline operator head", Perl, "my $x = q\n # comment\n {first\n last};", []FoldRange{{0, 3}}},
		{"perl quote rollback", Perl, "my %h = (q => 1);\nsub demo {\n work();\n}", []FoldRange{{1, 3}}},
		{"perl paired replacement", Perl, "$x =~ s{first\n }\n {second\n }g;", []FoldRange{{0, 3}}},
		{"perl unpaired replacement", Perl, "$x =~ s/first\n/second\n/g;", []FoldRange{{0, 2}}},
		{"perl regex flags", Perl, "my $rx = qr{first\n last}ims;", []FoldRange{{0, 1}}},
		{"perl regex trailing statement", Perl, "my $rx = qr{first\n last}ims; work();", nil},
		{"perl transliteration", Perl, "$x =~ tr{first\n }{second\n }d;", []FoldRange{{0, 2}}},
		{"perl incomplete replacement", Perl, "$x =~ s{first\n }{second", nil},
		{"perl adjacent literals", Perl, "my $x = q{first\n}; my $y = qq{second\n};", []FoldRange{{1, 2}}},
		{"perl heredocs", Perl, "say <<A, <<~'B';\n @end }\nA\n [ if\n B", []FoldRange{{0, 4}}},
		{"perl heredoc code", Perl, "sub demo {\n my $x = <<'EOF';\n } @end\nEOF\n}", []FoldRange{{0, 4}, {1, 3}}},
		{"perl pod", Perl, "=head1 Doc\nsub fake {\n=cut\nsub real {\n work();\n}", []FoldRange{{0, 2}, {3, 5}}},
		{"perl data", Perl, "__DATA__\nsub fake {\n}", nil},
		{"perl name only", Perl, "sub q {\n work();\n}\n$obj->s(1);", []FoldRange{{0, 2}}},
		{"perl replacement comments", Perl, "$x =~ s{first\n}\n # replacement follows\n {second\n }g;", []FoldRange{{0, 4}}},
		{"perl quoted heredoc incomplete", Perl, "my $x = <<'EOF';\n } [", nil},
		{"perl pod incomplete", Perl, "=head1 Doc\nsub fake {\n}", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := Builtin(tt.id)
			if l.Folding() == nil {
				t.Fatal("missing folding provider")
			}
			got, err := l.Folding()(context.Background(), tt.text)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("folds = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestProfileFoldingIncompleteAndTrailingCode(t *testing.T) {
	for _, id := range []ID{ObjectiveC, ObjectiveCpp, SCSS, Less, Perl} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			fold := l.Folding()
			for _, text := range []string{"value = {\n x", "value = {\n ]\n}"} {
				got, err := fold(context.Background(), text)
				if err != nil || len(got) != 0 {
					t.Fatalf("incomplete %q = %v, %v", text, got, err)
				}
			}
			got, err := fold(context.Background(), "value = {\n x\n} tail")
			if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 1}}) {
				t.Fatalf("trailing source = %v, %v", got, err)
			}
		})
	}
}

func TestPerlFoldingLongHeredoc(t *testing.T) {
	l, _ := Builtin(Perl)
	label := "TEXT" + strings.Repeat("x", 5000)
	for at := 4090; at <= 4097; at++ {
		pad := strings.Repeat(" ", at)
		text := pad + "my $x = <<~'" + label + "';\n } [ @end\n" + pad + label
		got, err := l.Folding()(context.Background(), text)
		if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 2}}) {
			t.Fatalf("offset %d: %v, %v", at, got, err)
		}
	}
}

func TestProfileFoldingTokenizerObservation(t *testing.T) {
	for _, tt := range []struct {
		id   ID
		text string
	}{
		{ObjectiveC, "@implementation Demo\n- (void)run { id x = @\"正文 }\"; }\n@end"},
		{ObjectiveCpp, "@implementation Demo\nauto x = R\"tag(\n@end 正文\n)tag\";\n@end"},
		{SCSS, ".demo {\n content: \"#{inspect((a: \"正文\"))}\";\n /* outer #{\"x\"} tail */\n}"},
		{Less, ".@{name} {\n @x: `({\n needle: '正文',\n })`;\n}"},
		{Perl, "my $x = s{正文\n}{other\n}gi;\nsay <<~'EOF';\n } [\n EOF\n=head1 Doc\ntext\n=cut"},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			public := l.Tokenizer()
			for chunk := 1; chunk <= 17; chunk++ {
				observed := foldingTokenizer{public, &foldLexemes{row: -1}}
				a, b := public.InitialState(), public.InitialState()
				for _, line := range strings.Split(tt.text, "\n") {
					for offset := 0; ; {
						in := Input{Text: line[offset:], Offset: offset, Limit: min(chunk, len(line)-offset), LineEnd: true}
						x, err := public.Scan(in, a)
						if err != nil {
							t.Fatal(err)
						}
						y, err := observed.Scan(in, b)
						if err != nil {
							t.Fatal(err)
						}
						a, b = x.State, y.State
						if !a.Equal(b) {
							t.Fatalf("chunk %d offset %d changed state", chunk, offset)
						}
						y.State = x.State
						if !reflect.DeepEqual(x, y) {
							t.Fatalf("chunk %d offset %d changed tokens: %v / %v", chunk, offset, x, y)
						}
						offset += x.Consumed
						if offset == len(line) {
							break
						}
						if x.Consumed <= 0 {
							t.Fatal("no progress")
						}
					}
				}
			}
		})
	}
}

func TestProfileFoldingFragmentsAndWorkers(t *testing.T) {
	for _, tt := range []struct {
		id          ID
		open, close string
	}{
		{ObjectiveC, "@implementation Demo", "@end"}, {ObjectiveCpp, "auto x = R\"tag(", ")tag\";"},
		{SCSS, "/*", "*/"}, {Less, "@x: `", "`;"}, {Perl, "my $x = qq{", "};"},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			if l.Folding() == nil {
				t.Fatal("missing folding provider")
			}
			fold := l.Folding()
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					for at := 4090; at <= 4097; at++ {
						pad := strings.Repeat(" ", at)
						text := strings.Repeat("\n", i) + pad + tt.open + "\n" + strings.Repeat("x", 9000) + "\n" + pad + tt.close
						got, err := fold(context.Background(), text)
						if err != nil || !reflect.DeepEqual(got, []FoldRange{{i, i + 2}}) {
							t.Errorf("worker %d offset %d: %v, %v", i, at, got, err)
						}
					}
				}(i)
			}
			wg.Wait()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			got, err := structuralFolding(tt.id, cancelCodeScanner{l.Tokenizer(), cancel})(ctx, strings.Repeat("x ", 16000))
			if err != context.Canceled || got != nil {
				t.Fatalf("mid-scan cancellation = %v, %v", got, err)
			}
		})
	}
}
