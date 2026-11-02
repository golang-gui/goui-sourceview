// JSX text and attribute values have different rules from JavaScript strings.
export function Greeting({ name, items, ...props }) {
  return (
    <section className="greeting" {...props}>
      <h1 title="Hello &amp; welcome">Hello, {name}!</h1>
      <>
        {items.map(item => <Item key={item.id} value={item} />)}
        {/* Comments belong inside a JavaScript expression. */}
        <p>Quotes, // and /* stay text here.</p>
      </>
    </section>
  );
}
