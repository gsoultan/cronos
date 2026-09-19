/**
 * Element construction without a framework.
 *
 * `innerHTML` is not used anywhere in this package. Report values arrive from
 * the server already formatted, and formatted is not the same as safe: a
 * customer name is data from *our* customer's database, so it is exactly the
 * text an attacker controls in a multi-tenant product. Building nodes and
 * setting textContent means there is no escaping to get right.
 */
export function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  attrs?: Record<string, string>,
  ...children: (Node | string)[]
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag)
  for (const [k, v] of Object.entries(attrs ?? {})) node.setAttribute(k, v)
  for (const c of children) node.append(c)
  return node
}

/**
 * Replaces a node's children in one operation.
 *
 * ParentNode rather than Element: a shadow root is not an element, and it is
 * the first thing this is called on.
 */
export function fill(parent: ParentNode, ...children: (Node | string)[]) {
  parent.replaceChildren(...children)
}
