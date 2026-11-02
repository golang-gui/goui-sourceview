# Original Elixir sample for SourceView.
defmodule Editor do
  @theme :modern

  def greet(name) do
    message = "Hello, #{name}!"
    literal = ~S"#{this_stays_literal}"
    pattern = ~r/[a-z_]+/i
    %{message: message, literal: literal, pattern: pattern, theme: @theme}
  end
end
