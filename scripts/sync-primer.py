#!/usr/bin/env python3
"""Generate the book and skill primer from the embedded CLI source."""
from pathlib import Path

root = Path(__file__).resolve().parents[1]
primer = (root / "internal/cli/primer.md").read_text()
(root / "docs/book/src/commands/prime.md").write_text(primer)
skill = root / "skills/desire-path/SKILL.md"
text = skill.read_text()
begin = "<!-- BEGIN dp prime -->\n"
end = "<!-- END dp prime -->"
head, tail = text.split(begin, 1)
_, tail = tail.split(end, 1)
skill.write_text(head + begin + primer + end + tail)
