using System;

class Hello {
    static void Main() {
        var name = "SourceView";
        var path = @"C:\code\SourceView";
        var message = $"Hello {name}! {1.25:0.00}";
        var json = $$"""
            { "name": "{{name}}", "path": "literal" }
            """;
        Console.WriteLine($"{message}\n{path}\n{json}");
    }
}
