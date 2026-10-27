#!/usr/bin/env python3
from dataclasses import dataclass


@dataclass
class Document:
    title: str
    editable: bool = True

    def greeting(self) -> str:
        """Build a message without modifying the document."""
        return f"Hello from {self.title!r}: editable={self.editable}"


if __name__ == "__main__":
    document = Document("SourceView")
    print(document.greeting())
    print(r"A raw path: C:\editor\samples")
