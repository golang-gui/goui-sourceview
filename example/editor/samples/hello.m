#import <Foundation/Foundation.h>

@interface Greeting : NSObject
@property(nonatomic, copy) NSString *name;
@end

@implementation Greeting
- (NSString *)message {
    return [NSString stringWithFormat:@"Hello, %@!", self.name];
}
@end

int main(void) {
    @autoreleasepool {
        Greeting *greeting = [Greeting new];
        greeting.name = @"SourceView";
        NSDictionary *settings = @{ @"enabled": @YES, @"count": @42 };
        NSLog(@"%@ %@", [greeting message], settings);
    }
    return 0;
}
