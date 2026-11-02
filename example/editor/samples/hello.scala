// Original Scala sample for SourceView.
object Greeting {
  /* Outer comment /* nested */ continues here. */
  def greet(name: String): String = {
    val message = s"Hello, $name! ${if (name.nonEmpty) "Welcome" else "Later"}"
    val literal = raw"No newline escape: \n, but $name still interpolates"
    val block = s"""
      |$message
      |${List(1, 2, 3).mkString(", ")}
      |$$ remains a dollar sign.
      |""".stripMargin
    println(literal)
    block
  }
}
