class Greeting
  def initialize(name)
    @name = name
  end

  def message
    <<~TEXT
      Hello, #{@name}!
      Nested expression: #{true ? %Q{welcome #{@name}} : 'later'}
    TEXT
  end
end

# Percent literals and heredocs keep their own delimiter rules.
names = %w[SourceView Ruby]
pattern = %r{[a-z]+}i
puts Greeting.new(names.first).message
puts(<<FIRST, <<-'SECOND')
Interpolation: #{names.join(', ')}
FIRST
Literal: #{names}
  SECOND
