// Forum frontend: no framework, just DOM + fetch.

const state = {
  user: null,        // {id, email, username} | null
  categories: [],
  filter: { type: "all" }, // {type: "all"|"category"|"mine"|"liked", value?}
};

const el = (sel) => document.querySelector(sel);
const els = (sel) => Array.from(document.querySelectorAll(sel));

function escapeHTML(str) {
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
}

function formatDate(iso) {
  const d = new Date(iso);
  return d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

function showAlert(message) {
  const box = el("#alert");
  box.textContent = message;
  box.classList.remove("hidden");
  setTimeout(() => box.classList.add("hidden"), 4000);
}

// ---- auth area ------------------------------------------------------------

function renderAuthArea() {
  const area = el("#auth-area");
  if (state.user) {
    area.innerHTML = `
      <span class="username">${escapeHTML(state.user.username)}</span>
      <button class="btn" id="logout-btn">Log out</button>`;
    el("#logout-btn").addEventListener("click", async () => {
      await api.logout();
      state.user = null;
      renderAuthArea();
      renderUserFilters();
      loadAndRenderPosts();
    });
  } else {
    area.innerHTML = `<button class="btn" id="login-btn">Log in</button>`;
    el("#login-btn").addEventListener("click", () => openAuthModal("login"));
  }
  el("#new-post-btn").classList.toggle("hidden", !state.user);
}

function renderUserFilters() {
  el("#user-filters").classList.toggle("hidden", !state.user);
}

// ---- auth modal ------------------------------------------------------------

function openAuthModal(which) {
  el("#login-form").classList.toggle("hidden", which !== "login");
  el("#register-form").classList.toggle("hidden", which !== "register");
  el("#login-error").textContent = "";
  el("#register-error").textContent = "";
  el("#auth-modal").showModal();
}

el("#show-register").addEventListener("click", (e) => { e.preventDefault(); openAuthModal("register"); });
el("#show-login").addEventListener("click", (e) => { e.preventDefault(); openAuthModal("login"); });

els("[data-close-modal]").forEach((btn) => {
  btn.addEventListener("click", () => btn.closest("dialog").close());
});

el("#login-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = new FormData(e.target);
  try {
    await api.login({ email: form.get("email"), password: form.get("password") });
    state.user = await api.me();
    el("#auth-modal").close();
    renderAuthArea();
    renderUserFilters();
    loadAndRenderPosts();
  } catch (err) {
    el("#login-error").textContent = err.message;
  }
});

el("#register-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = new FormData(e.target);
  try {
    await api.register({
      email: form.get("email"),
      username: form.get("username"),
      password: form.get("password"),
    });
    state.user = await api.me();
    el("#auth-modal").close();
    renderAuthArea();
    renderUserFilters();
    loadAndRenderPosts();
  } catch (err) {
    el("#register-error").textContent = err.message;
  }
});

// ---- categories / filters ---------------------------------------------------

async function loadCategories() {
  state.categories = await api.categories();
  const list = el("#category-list");
  // Keep the "All Posts" entry, append one button per category.
  list.querySelectorAll("[data-category-id]").forEach((n) => n.remove());
  for (const cat of state.categories) {
    const li = document.createElement("li");
    li.innerHTML = `<button class="filter-btn" data-category-id="${cat.id}">${escapeHTML(cat.name)}</button>`;
    list.appendChild(li);
  }
  populatePostCategoryCheckboxes();
  bindFilterButtons();
}

function populatePostCategoryCheckboxes() {
  const fieldset = el("#post-categories");
  fieldset.querySelectorAll("label").forEach((n) => n.remove());
  for (const cat of state.categories) {
    const label = document.createElement("label");
    label.innerHTML = `<input type="checkbox" name="category" value="${cat.id}"> ${escapeHTML(cat.name)}`;
    fieldset.appendChild(label);
  }
}

function bindFilterButtons() {
  els(".filter-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      els(".filter-btn").forEach((b) => b.classList.remove("active"));
      btn.classList.add("active");

      if (btn.dataset.categoryId) {
        state.filter = { type: "category", value: btn.dataset.categoryId };
      } else if (btn.dataset.filter === "mine") {
        state.filter = { type: "mine" };
      } else if (btn.dataset.filter === "liked") {
        state.filter = { type: "liked" };
      } else {
        state.filter = { type: "all" };
      }
      loadAndRenderPosts();
    });
  });
}

// ---- posts -------------------------------------------------------------------

function filterParams() {
  switch (state.filter.type) {
    case "category": return { category: state.filter.value };
    case "mine": return { mine: "1" };
    case "liked": return { liked: "1" };
    default: return undefined;
  }
}

async function loadAndRenderPosts() {
  const list = el("#post-list");
  list.innerHTML = `<p class="empty-state">Loading…</p>`;
  try {
    const posts = await api.posts(filterParams());
    if (posts.length === 0) {
      list.innerHTML = `<p class="empty-state">Nothing here yet.</p>`;
      return;
    }
    list.innerHTML = "";
    for (const post of posts) list.appendChild(renderPostCard(post));
  } catch (err) {
    list.innerHTML = "";
    showAlert(err.message);
  }
}

function renderPostCard(post) {
  const card = document.createElement("article");
  card.className = "post-card";
  const excerpt = post.body.length > 220 ? post.body.slice(0, 220) + "…" : post.body;
  card.innerHTML = `
    <h3>${escapeHTML(post.title)}</h3>
    <div class="post-meta">by ${escapeHTML(post.username)} · ${formatDate(post.created_at)}</div>
    <div class="tag-list">${post.categories.map((c) => `<span class="tag">${escapeHTML(c.name)}</span>`).join("")}</div>
    <p class="post-excerpt">${escapeHTML(excerpt)}</p>
    <div class="post-stats">
      <span>▲ ${post.likes} · ▼ ${post.dislikes}</span>
    </div>`;
  card.addEventListener("click", () => openPostDetail(post.id));
  return card;
}

// ---- new post modal ------------------------------------------------------------

el("#new-post-btn").addEventListener("click", () => {
  el("#post-form").reset();
  el("#post-error").textContent = "";
  el("#post-modal").showModal();
});

el("#post-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = new FormData(e.target);
  const categoryIds = els('#post-categories input[type=checkbox]:checked').map((i) => Number(i.value));
  try {
    const created = await api.createPost({
      title: form.get("title"),
      body: form.get("body"),
      category_ids: categoryIds,
    });
    el("#post-modal").close();
    loadAndRenderPosts();
    openPostDetail(created.id);
  } catch (err) {
    el("#post-error").textContent = err.message;
  }
});

// ---- post detail + comments ------------------------------------------------------

async function openPostDetail(id) {
  const container = el("#detail-content");
  container.innerHTML = `<p class="empty-state">Loading…</p>`;
  el("#detail-modal").showModal();

  try {
    const [post, comments] = await Promise.all([api.post_(id), api.comments(id)]);
    container.innerHTML = renderPostDetail(post, comments);
    bindDetailEvents(post, comments);
  } catch (err) {
    container.innerHTML = `<p class="empty-state">${escapeHTML(err.message)}</p>`;
  }
}

function renderPostDetail(post, comments) {
  const commentsHTML = comments.map((c) => `
    <div class="comment" data-comment-id="${c.id}">
      <div class="comment-meta">${escapeHTML(c.username)} · ${formatDate(c.created_at)}</div>
      <p>${escapeHTML(c.body)}</p>
      <div class="reaction-controls">
        <button class="react-btn comment-like ${c.user_reaction === 1 ? "active-like" : ""}">▲ ${c.likes}</button>
        <button class="react-btn comment-dislike ${c.user_reaction === -1 ? "active-dislike" : ""}">▼ ${c.dislikes}</button>
      </div>
    </div>`).join("") || `<p class="empty-state">No comments yet.</p>`;

  return `
    <h2 style="font-family:var(--serif);font-weight:400">${escapeHTML(post.title)}</h2>
    <div class="post-meta">by ${escapeHTML(post.username)} · ${formatDate(post.created_at)}</div>
    <div class="tag-list">${post.categories.map((c) => `<span class="tag">${escapeHTML(c.name)}</span>`).join("")}</div>
    <p>${escapeHTML(post.body)}</p>
    <div class="reaction-controls" id="post-reactions">
      <button class="react-btn post-like ${post.user_reaction === 1 ? "active-like" : ""}">▲ Like (${post.likes})</button>
      <button class="react-btn post-dislike ${post.user_reaction === -1 ? "active-dislike" : ""}">▼ Dislike (${post.dislikes})</button>
    </div>
    <h3 style="font-family:var(--serif);font-weight:400;margin-top:1.5rem">Comments</h3>
    <div id="comment-list">${commentsHTML}</div>
    ${state.user ? `
      <form id="comment-form" class="comment-form">
        <textarea name="body" rows="2" placeholder="Add a comment…" required></textarea>
        <button type="submit" class="btn btn-primary">Post</button>
      </form>` : `<p class="empty-state">Log in to comment.</p>`}
  `;
}

function bindDetailEvents(post, comments) {
  if (state.user) {
    el("#post-reactions .post-like").addEventListener("click", () => react("post", post.id));
    el("#post-reactions .post-dislike").addEventListener("click", () => react("post", post.id, -1));

    els(".comment").forEach((node) => {
      const cid = node.dataset.commentId;
      node.querySelector(".comment-like").addEventListener("click", () => react("comment", cid, 1, post.id));
      node.querySelector(".comment-dislike").addEventListener("click", () => react("comment", cid, -1, post.id));
    });

    const form = el("#comment-form");
    if (form) {
      form.addEventListener("submit", async (e) => {
        e.preventDefault();
        const body = new FormData(e.target).get("body");
        try {
          await api.createComment(post.id, body);
          openPostDetail(post.id); // refresh detail view
          loadAndRenderPosts();     // refresh counts in list
        } catch (err) {
          showAlert(err.message);
        }
      });
    }
  } else {
    els(".react-btn").forEach((b) => b.setAttribute("disabled", "true"));
  }
}

async function react(kind, id, value = 1, postId) {
  try {
    if (kind === "post") {
      await api.reactToPost(id, value);
      openPostDetail(id);
    } else {
      await api.reactToComment(id, value);
      openPostDetail(postId);
    }
    loadAndRenderPosts();
  } catch (err) {
    showAlert(err.message);
  }
}

// ---- boot ------------------------------------------------------------------

(async function init() {
  state.user = await api.me();
  renderAuthArea();
  renderUserFilters();
  await loadCategories();
  loadAndRenderPosts();
})();
