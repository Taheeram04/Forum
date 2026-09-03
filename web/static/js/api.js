// Thin wrapper around fetch for the forum's JSON API. Cookies (the
// session) are sent automatically via `credentials: "same-origin"`.
const api = {
  async request(method, path, body) {
    const res = await fetch(path, {
      method,
      credentials: "same-origin",
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    });

    let data = null;
    const text = await res.text();
    if (text) {
      try { data = JSON.parse(text); } catch { /* non-JSON response */ }
    }

    if (!res.ok) {
      const message = (data && data.error) || `request failed (${res.status})`;
      throw new Error(message);
    }
    return data;
  },

  get(path) { return this.request("GET", path); },
  post(path, body) { return this.request("POST", path, body); },

  me() { return this.request("GET", "/api/me").catch(() => null); },
  register(payload) { return this.post("/api/register", payload); },
  login(payload) { return this.post("/api/login", payload); },
  logout() { return this.post("/api/logout"); },

  categories() { return this.get("/api/categories"); },

  posts(params) {
    const qs = params ? "?" + new URLSearchParams(params).toString() : "";
    return this.get("/api/posts" + qs);
  },
  post_(id) { return this.get(`/api/posts/${id}`); },
  createPost(payload) { return this.post("/api/posts", payload); },

  comments(postId) { return this.get(`/api/posts/${postId}/comments`); },
  createComment(postId, body) { return this.post(`/api/posts/${postId}/comments`, { body }); },

  reactToPost(id, value) { return this.post(`/api/posts/${id}/react`, { value }); },
  reactToComment(id, value) { return this.post(`/api/comments/${id}/react`, { value }); },
};
