# Original R sample for SourceView; raw strings require R 4.0 or newer.
greet <- function(name) {
  message <- paste("Hello,", name)
  settings <- r"--{
    {"enabled": true, "path": "C:\SourceView", "quote": "raw"}
  }--"
  list(message = message, settings = settings)
}

`display name` <- "SourceView"
values <- c(1L, 2.5, 0x1.fp+2, NA_real_, Inf)
positive <- values[!is.na(values)] |> sum()
print(greet(`display name`))
print(positive %in% c(1, 2, 3))
