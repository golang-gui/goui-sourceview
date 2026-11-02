// Original F# sample for SourceView.
(* Nested (* comments *) retain their lexical state. *)
let languages = ["JSON"; "XML"; "HTML"]
let describe name =
    let message = $"Hello, {name}!"
    let path = @"C:\SourceView\editor"
    let banner = """A native
code editor"""
    printfn "%s %s %s" message path banner

languages |> List.iter describe
