@file:JvmName("SourceViewExample")

/* Outer comment /* nested comment */ still a comment. */
fun main() {
    val name = "SourceView"
    val message = "Hello, $name! ${if (name.isNotEmpty()) "Welcome" else "Later"}"
    val json = $$"""
        {
          "$schema": "literal dollar sign",
          "name": "$${name}",
          "editable": true
        }
    """.trimIndent()
    println(message)
    println(json)
}
