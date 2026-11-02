(* Original OCaml sample for SourceView.
   Nested (* comments *) preserve their state. *)
let identity (value : 'a) : 'a = value

let greet name =
  let theme = "Modern" in
  let literal = {editor|A native "code" editor|editor} in
  Printf.sprintf "Hello, %s! %s / %s" name theme literal

let () = List.iter (fun name -> print_endline (greet name))
  ["JSON"; "XML"; "HTML"]
