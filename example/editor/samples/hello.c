#include <stdio.h>
#define SQUARE(x) ((x) * (x))

/* Original C sample for SourceView. */
int main(void) {
    const char *message = u8"Hello, SourceView!\n";
    double value = 0x1.fp+2;
    printf("%svalue = %.2f, square = %d\n", message, value, SQUARE(3));
    return 0;
}
