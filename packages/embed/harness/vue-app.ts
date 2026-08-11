/*
 * The same element in Vue, to check the framework-agnostic claim rather than
 * assert it.
 *
 * Render functions rather than templates: `isCustomElement` only affects Vue's
 * template *compiler*, while the decision that actually matters — whether
 * `filters` is set as a property or stringified into an attribute — is made by
 * the runtime, and h() exercises exactly that path.
 */
import { createApp, h, ref } from 'vue'
import '../dist/cronos-embed.js'

const status = ref<string | null>(null)

createApp({
  render() {
    return h('main', [
      h('button', {
        'data-testid': 'filter',
        onClick: () => (status.value = 'overdue'),
      }, 'Overdue only'),
      h('cronos-report', {
        endpoint: location.origin,
        token: 'tok',
        report: 'monthly',
        filters: status.value ? { status: { op: 'eq', values: [status.value] } } : {},
      }),
    ])
  },
}).mount('#root')
