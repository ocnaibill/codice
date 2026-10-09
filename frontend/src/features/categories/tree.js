// The tree of categories as the screens read it (DEC-140). The server sends it flat, sorted by name, each with its parent; here it is
// put in reading order, with how deep each one is, and asked what can go where. The limit of levels is the server's (it refuses more).

export const MAX_DEPTH = 3;

const idOf = (c) => c.id;

/** The categories in reading order (each, then what is under it), each with `depth` (1 at the top). One whose parent is not in the list is at the top. */
export function inOrder(flat) {
  const known = new Set(flat.map(idOf));
  const children = new Map();
  for (const c of flat) {
    const key = c.parentId != null && known.has(c.parentId) ? c.parentId : null;
    if (!children.has(key)) children.set(key, []);
    children.get(key).push(c);
  }
  const out = [];
  const walk = (parent, depth) => {
    for (const c of children.get(parent) ?? []) {
      out.push({ ...c, depth, hasChildren: children.has(c.id) });
      walk(c.id, depth + 1);
    }
  };
  walk(null, 1);
  return out;
}

/** The ids of what is under a category, at any depth (not the category itself). */
export function descendantsOf(flat, id) {
  const under = new Set();
  let grew = true;
  while (grew) {
    grew = false;
    for (const c of flat) {
      if (c.parentId != null && (c.parentId === id || under.has(c.parentId)) && !under.has(c.id)) {
        under.add(c.id);
        grew = true;
      }
    }
  }
  return under;
}

/** How many levels the category and what is under it have (1 for one with nothing under it). */
export function heightOf(flat, id) {
  const levels = (of) => 1 + Math.max(0, ...flat.filter((c) => c.parentId === of).map((c) => levels(c.id)));
  return levels(id);
}

/** The places a category can be put: the top (null) and every category that is not it nor under it, and that leaves the tree within the limit. */
export function placesFor(flat, id) {
  const ordered = inOrder(flat);
  const barred = new Set([id, ...descendantsOf(flat, id)]);
  const height = id == null ? 1 : heightOf(flat, id);
  return ordered.filter((c) => !barred.has(c.id) && c.depth + height <= MAX_DEPTH);
}

/** "Mangá › Seinen": where a category is, from the top. */
export function pathOf(flat, id) {
  const byId = new Map(flat.map((c) => [c.id, c]));
  const names = [];
  for (let c = byId.get(id); c; c = c.parentId != null ? byId.get(c.parentId) : undefined) names.unshift(c.name);
  return names.join(' › ');
}

/** The categories above one, from the top down (not the category itself). */
export function ancestorsOf(flat, id) {
  const byId = new Map(flat.map((c) => [c.id, c]));
  const above = [];
  for (let c = byId.get(byId.get(id)?.parentId); c; c = byId.get(c.parentId)) above.unshift(c);
  return above;
}
