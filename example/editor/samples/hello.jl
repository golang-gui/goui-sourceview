# Original Julia sample for SourceView.
#= Nested #= comments =# preserve their state. =#
function greet(name)
    message = "Hello, $(uppercase(name))!"
    literal = raw"literal $name"
    banner = """A native
    code editor"""
    values = [1 2; 3 4]
    transposed = values'
    return (message, literal, banner, transposed)
end
