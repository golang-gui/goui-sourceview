import 'dart:convert';

/* Outer comment /* nested */ continues here. */
class Greeting {
  final String name;
  const Greeting({required this.name});

  @override
  String toString() => 'Hello, $name!';
}

void main() {
  const greeting = Greeting(name: 'SourceView');
  final message = '''
    ${greeting.toString()}
    ${jsonEncode({'enabled': true, 'count': 1_000})}
  ''';
  final raw = r'No interpolation: $name and no escapes: \n';
  print(message);
  print(raw);
}
