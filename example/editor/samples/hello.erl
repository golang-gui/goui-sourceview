% Original Erlang sample for SourceView.
-module(editor).
-export([greet/1]).

greet(Name) ->
    Theme = modern,
    Values = [16#FF, $a, $\n, 'quoted atom'],
    case Name of
        undefined -> {error, "Missing name"};
        _ -> {ok, Name, Theme, Values}
    end.
