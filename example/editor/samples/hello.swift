import Foundation

/* Outer comment /* nested */ continues here. */
@MainActor
func greet(_ name: String) -> String {
    let message = "Hello, \(name)!"
    let raw = #"A raw quote: " and an interpolation: \#(name)"#
    let multiline = """
        Swift supports multiline strings.
        \(message)
        """
    let identifier = #/[A-Za-z_][A-Za-z_0-9]*/#
    print(raw, identifier)
    return multiline
}

print(greet("SourceView"))
