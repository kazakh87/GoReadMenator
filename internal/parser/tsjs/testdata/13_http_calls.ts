// SZENARIO: Frontend-Aufrufe zum Backend: fetch mit Literal, Verkettung, Template und Optionsobjekt; axios und http-Client
// ABDECKUNG: fetch mit Literal, fetch mit Verkettung, fetch mit Template-Literal, method-Option, axios.post, http.get, URL mit Query-String, dynamische URL ohne Literal bleibt ohne Verweis
export async function a() { return fetch("/api/orders"); }
export async function b(id: number) { return fetch("/api/orders/" + id); }
export async function c(id: number) { return fetch(`/api/orders/${id}?x=1`); }
export async function d(body: string) { return fetch("/api/orders", { method: "POST", body }); }
export async function e(id: number) { return axios.post("/api/orders/" + id, {}); }
export async function f(id: number) { return http.get("/api/items/" + id); }
export async function g(url: string) { return fetch(url); }
