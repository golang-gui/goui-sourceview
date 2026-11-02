#import <Foundation/Foundation.h>
#include <string>
#include <vector>

template<class T>
static NSNumber *box(T value) {
    return @(value);
}

int main() {
    @autoreleasepool {
        const std::string message = R"text(Hello, "SourceView"!
@interface here is literal text, not a directive.)text";
        std::vector<int> values{1, 2, 3};
        NSArray *items = @[box(values[0]), @YES];
        NSLog(@"%s %@", message.c_str(), items);
    }
    return 0;
}
