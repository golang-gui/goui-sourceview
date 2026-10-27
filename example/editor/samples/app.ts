interface DocumentInfo {
  readonly title: string;
  editable: boolean;
  tags?: string[];
}

type DocumentKey = keyof DocumentInfo;

export function property<T extends DocumentInfo, K extends keyof T>(
  document: T, key: K,
): T[K] {
  return document[key];
}

const document: DocumentInfo = { title: "SourceView", editable: true };
console.log(`Title: ${property(document, "title")}`);
