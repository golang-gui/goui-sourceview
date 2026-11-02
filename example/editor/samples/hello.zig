//! Original Zig sample for SourceView.
const std = @import("std");

pub fn main() void {
    const name = "SourceView";
    const message =
        \\A multiline string keeps \n and {braces} literally.
        \\Each line has its own prefix.
    ;
    const @"wide integer": u128 = 0x1234_abcd;
    std.debug.print("Hello, {s}! {s} {d}\n", .{
        name, message, @"wide integer",
    });
}
