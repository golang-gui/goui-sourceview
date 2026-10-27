// Templates, regular expressions and division use different lexical contexts.
export function describe(document) {
  const words = document.title.match(/[\p{L}\d_-]+/gu) ?? [];
  const ratio = words.length / 2;
  return `Title: ${document.title}
Words: ${words.length}, ratio: ${ratio}`;
}

const document = { title: "GOUI SourceView", editable: true };
console.log(describe(document));
