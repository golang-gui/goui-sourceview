#include <iostream>
#include <string_view>

// Quotes and comment markers inside a raw string stay literal.
constexpr std::string_view message = R"text(
    { "name": "SourceView", "note": "/* literal */" }
)text";

int main() {
    auto count = 1'000ULL;
    std::cout << message << '\n' << count << '\n';
    return 0;
}
