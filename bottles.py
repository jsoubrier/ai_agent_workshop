#!/usr/bin/env python3
"""Print the full lyrics of "99 Bottles of Beer" to stdout."""


def quantity(n: int) -> str:
    """The count, as it appears in the song: 0 is a word, not a digit."""
    return "no more" if n == 0 else str(n)


def noun(n: int) -> str:
    """Only one bottle is singular. Zero bottles is plural."""
    return "bottle" if n == 1 else "bottles"


def beer(n: int) -> str:
    """e.g. '99 bottles of beer', '1 bottle of beer', 'no more bottles of beer'."""
    return f"{quantity(n)} {noun(n)} of beer"


def verse(n: int) -> list[str]:
    """One two-line verse, without trailing newlines."""
    if n == 0:
        return [
            f"{beer(0).capitalize()} on the wall, {beer(0)}.",
            f"Go to the store and buy some more, {beer(99)} on the wall.",
        ]
    return [
        f"{beer(n)} on the wall, {beer(n)}.",
        f"Take one down and pass it around, {beer(n - 1)} on the wall.",
    ]


def song(start: int = 99) -> list[str]:
    """Every verse from `start` down to 0, blank line between each."""
    verses = [verse(n) for n in range(start, -1, -1)]
    lines: list[str] = []
    for i, v in enumerate(verses):
        if i:
            lines.append("")
        lines.extend(v)
    return lines


def main() -> None:
    print("\n".join(song()))


if __name__ == "__main__":
    main()
