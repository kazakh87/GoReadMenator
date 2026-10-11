export async function loadOrders() {
  return fetch("/api/orders");
}

export async function loadOrder(id: number) {
  return fetch("/api/orders/" + id);
}

export async function loadOrderTpl(id: number) {
  return fetch(`/api/orders/${id}`);
}

export async function createOrder(body: string) {
  return fetch("/api/orders", { method: "POST", body });
}

export async function reports() {
  return fetch("/api/reports/summary");
}

export async function shared() {
  return fetch("/api/shared/item");
}

export async function health() {
  return fetch("/api/health?verbose=1");
}

export async function clearCache(key: string) {
  return fetch("/api/cache/" + key, { method: "DELETE" });
}

export async function viaAxios(id: number) {
  return axios.post("/api/orders/" + id, {});
}
