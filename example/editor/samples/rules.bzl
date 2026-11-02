# Original Starlark sample for SourceView.
load("//tools:helpers.bzl", "make_targets")

def editor_sources(name, srcs):
    """Creates a source group for an editor example."""
    selected = [src for src in srcs if src.endswith(".go")]
    make_targets(
        name = name,
        srcs = selected,
        tags = ["native", "editor"],
    )

pattern = r"[a-z]+\.[a-z]+"
message = "Hello, %s" % "SourceView"
