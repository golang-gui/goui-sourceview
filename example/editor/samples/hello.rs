fn first<'a>(items: &'a [&'a str]) -> Option<&'a str> {
    /* Outer comment /* nested comment */ still a comment. */
    items.first().copied()
}

fn main() {
    let r#type = r##"Raw "quotes" and a single "# stay literal."##;
    let letter = '字';
    match first(&["SourceView", "Rust"]) {
        Some(name) => println!("{name}: {r#type} {letter}"),
        None => println!("No items"),
    }
}
