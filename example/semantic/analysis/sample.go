package analysis

import (
	"image/color"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/style"
)

const Sample = `package main

type Person struct {
    Name string
    Age  int
}

func (p Person) Greeting() string { return "Hello " + p.Name }

func main() {
    value := "hello"
    person := Person{Name: "Ada", Age: 37}
    println(value, person.Greeting())
    // Change "hello" to 42: value changes semantic color at both locations.
    // Type person. below to get members from its checked type.
    // F2 requests scope completion, including your own declarations.
}
`

// Rules supplies application-specific semantic colors through the normal style
// system. Other syntax and chrome continue to follow Basic/Modern.
func Rules(dark bool) []style.Rule {
	colors := []color.NRGBA{{0x13, 0x79, 0x83, 255}, {0x79, 0x52, 0x15, 255}, {0x25, 0x74, 0x42, 255}, {0xd5, 0x2a, 0x91, 255}, {0x2d, 0x7e, 0xea, 255}, {0x8a, 0x54, 0x13, 255}, {0x76, 0x32, 0xc0, 255}, {0xd0, 0x20, 0x20, 255}}
	if dark {
		colors = []color.NRGBA{{0x4e, 0xc9, 0xb0, 255}, {0xdc, 0xdc, 0xaa, 255}, {0x9c, 0xdc, 0xfe, 255}, {0xf0, 0x7a, 0xc4, 255}, {0x69, 0xa7, 0xff, 255}, {0xce, 0x91, 0x78, 255}, {0xc5, 0x86, 0xc0, 255}, {0xff, 0x6b, 0x6b, 255}}
	}
	kinds := []language.Kind{Type, Function, Field, StringVariable, NumberVariable, Variable, Constant, Invalid}
	var rules []style.Rule
	for i, kind := range kinds {
		rules = append(rules, style.Name("source-view").Part("token-"+kind.String()).ForegroundColor(colors[i]))
	}
	return rules
}
