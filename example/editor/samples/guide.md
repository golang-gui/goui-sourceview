# SourceView

Edit **Markdown**, select text, fold headings and search across lines.

- [x] Markup first
- [ ] Press Enter to continue a list

> Quoted text keeps its prefix.

## Configuration

```json
{
  "editor": "SourceView",
  "folding": true
}
```

| Format | Support |
| --- | --- |
| XML / HTML | Tags and folding |
| Markdown | Headings and fenced languages |

## Embedded code

```go
func greeting(name string) string {
    return "Hello, " + name
}
```

```python
message = f"Hello from {'SourceView'}"
```

```typescript
interface Document { readonly title: string; }
const document: Document = { title: "SourceView" };
```
