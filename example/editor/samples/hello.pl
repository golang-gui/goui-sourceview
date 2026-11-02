#!/usr/bin/env perl
use strict;
use warnings;

my $name = 'SourceView';
my $message = qq{Hello, $name!};
my @words = qw(native code editor);
$message =~ s{Hello}{Welcome}g;
my $identifier = qr{[A-Za-z_][A-Za-z_0-9]*};

print <<~TEXT;
    $message
    Quoted operators and heredocs retain their own lexical state.
    TEXT

=pod
This documentation is not executable Perl code.
=cut

print join(' ', @words), "\n";
__DATA__
This data section is not scanned as code.
