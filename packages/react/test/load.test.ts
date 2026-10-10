import { describe, expect, test } from 'bun:test'
import { scriptURL } from '../src/load'

/* `endpoint` decides which code runs on the host's page now, not only where
   its data comes from. These are the shapes that would run something other
   than cronos's element. */
describe('the element is loaded only from an http(s) endpoint', () => {
  test.each([
    ['data:text/javascript,alert(1)', 'data:'],
    ['blob:https://app.acme.example/1d2c', 'blob:'],
    ['javascript:alert(1)', 'javascript:'],
    ['file:///etc/passwd', 'file:'],
  ])('%s is refused', (endpoint, scheme) => {
    expect(() => scriptURL(endpoint)).toThrow(scheme)
  })

  test('a relative endpoint is refused rather than resolved against the page', () => {
    expect(() => scriptURL('/reports')).toThrow('absolute URL')
  })
})

describe('the element comes from where the API is', () => {
  test('a server at its root', () => {
    expect(scriptURL('https://reports.acme.example'))
      .toBe('https://reports.acme.example/v1/embed/cronos-embed.js')
  })

  test('a trailing slash does not double', () => {
    expect(scriptURL('https://reports.acme.example/'))
      .toBe('https://reports.acme.example/v1/embed/cronos-embed.js')
  })

  test('a server behind a path prefix, joined as the element joins its API calls', () => {
    expect(scriptURL('https://acme.example/reports'))
      .toBe('https://acme.example/reports/v1/embed/cronos-embed.js')
  })
})
