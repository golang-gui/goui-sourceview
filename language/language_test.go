package language

import "testing"

func TestLookupDetached(t *testing.T) {
	a, ok := Lookup(XML)
	if !ok || a.Name != "XML" {
		t.Fatal(a, ok)
	}
	a.Extensions[0] = ".broken"
	a.MIMETypes[0] = "broken"
	b, _ := Lookup(XML)
	if b.Extensions[0] != ".xml" || b.MIMETypes[0] != "application/xml" {
		t.Fatal(b)
	}
	if _, ok := Lookup("unknown"); ok {
		t.Fatal("accepted unknown language")
	}
}

func TestGuess(t *testing.T) {
	for _, tc := range []struct {
		name, sample string
		want         ID
	}{
		{"a.JSON", "", JSON}, {"a.xhtml", "<html>", XML},
		{"a.html", "<?xml", HTML}, {"", " \xef\xbb\xbf", Plain},
		{"", "\xef\xbb\xbf <?xml version='1.0'?>", XML},
		{"", " <!DOCTYPE HTML>", HTML}, {"", "{\"a\":1}", JSON},
		{"", "words", Plain},
	} {
		if got := Guess(tc.name, []byte(tc.sample)); got != tc.want {
			t.Errorf("%q %q: %q, want %q", tc.name, tc.sample, got, tc.want)
		}
	}
}

func TestKind(t *testing.T) {
	if KindInvalid.String() != "invalid" || Kind(255).Valid() {
		t.Fatal("kind contract")
	}
}

func TestConfigurationAndShellDetection(t *testing.T) {
	for _, tc := range []struct {
		name, sample string
		want         ID
	}{
		{"Cargo.TOML", "", TOML}, {"config.YAML", "", YAML}, {".ci.yml", "", YAML},
		{"run.sh", "", Shell}, {"run.bash", "", Shell}, {"/home/user/.bashrc", "", Shell}, {".profile", "", Shell},
		{"script", "#!/bin/sh\necho hi", Shell}, {"script", "#!/usr/bin/env bash\n", Shell}, {"script", "#!/usr/bin/env -S bash -eu\n", Shell},
		{"x.json", "#!/bin/sh", JSON}, {"unknown", "#!/usr/bin/env python\n", Plain},
		{"unknown", " #!/bin/sh", Plain}, {".env", "NAME=value", Plain}, {"unknown", "name: value", Plain},
		{"unknown", string(make([]byte, 4096)) + "#!/bin/sh", Plain},
	} {
		if got := Guess(tc.name, []byte(tc.sample)); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.name, got, tc.want)
		}
	}
	for _, id := range []ID{TOML, YAML, Shell} {
		info, ok := Lookup(id)
		if !ok || info.Name == "" || len(info.Extensions) == 0 {
			t.Fatal(id, info, ok)
		}
	}
	if !KindVariable.Valid() || KindVariable.String() != "variable" {
		t.Fatal("variable style missing")
	}
}
