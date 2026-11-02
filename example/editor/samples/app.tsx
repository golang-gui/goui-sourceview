interface Item {
  readonly id: number;
  name: string;
}

// The comma distinguishes this generic arrow function from a JSX element.
const identity = <T,>(value: T): T => value;

export function ItemList({ items }: { items: Item[] }) {
  return (
    <ul className="items" aria-label="Items">
      {items.map(item => (
        <li key={item.id}>{identity(item.name)}</li>
      ))}
    </ul>
  );
}
