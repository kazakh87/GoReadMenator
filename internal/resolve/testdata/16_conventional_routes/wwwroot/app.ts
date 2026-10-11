export async function product() { return fetch("/products/5"); }
export async function details() { return fetch("/orders/details/5"); }
export async function listAll() { return fetch("/orders/listall"); }
export async function privacy() { return fetch("/home/privacy"); }
export async function legacy() { return fetch("/api/legacy/items"); }
export async function helper() { return fetch("/orders/helper"); }
