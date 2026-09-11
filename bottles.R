#!/usr/bin/env Rscript

# "99 Bottles of Beer" — the whole song, to stdout.
# The interesting part is the bottom: 1 is singular, and 0 is "no more".

# Count as a word: 0 is "no more", everything else is the number.
count_word <- function(n) if (n == 0L) "no more" else as.character(n)

# "bottle" vs "bottles" — only 1 is singular; zero takes the plural.
bottles <- function(n) if (n == 1L) "bottle" else "bottles"

# e.g. "99 bottles of beer", "1 bottle of beer", "no more bottles of beer"
phrase <- function(n) paste0(count_word(n), " ", bottles(n), " of beer")

verse <- function(n) {
  if (n == 0L) {
    # The wrap-around verse: capitalised at the start of the line.
    first <- sub("^no more", "No more", phrase(0L))
    c(
      paste0(first, " on the wall, ", phrase(0L), "."),
      paste0("Go to the store and buy some more, ", phrase(99L), " on the wall.")
    )
  } else {
    c(
      paste0(phrase(n), " on the wall, ", phrase(n), "."),
      paste0(
        "Take ", if (n == 1L) "it" else "one",
        " down and pass it around, ", phrase(n - 1L), " on the wall."
      )
    )
  }
}

lines <- character(0)
for (n in 99:0) {
  if (length(lines) > 0L) lines <- c(lines, "")  # blank line between verses
  lines <- c(lines, verse(n))
}

# `sep` is appended after the last element too, so this already ends the
# final line — no extra cat("\n") (that would emit a trailing blank line).
cat(lines, sep = "\n")
