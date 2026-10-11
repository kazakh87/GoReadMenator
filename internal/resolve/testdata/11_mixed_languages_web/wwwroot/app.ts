export function load(id: number) {
  return fetch("/api/orders/" + id);
}
