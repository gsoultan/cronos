import { expect, test } from 'bun:test'
import { codeProblem, emailProblem, passwordProblem } from './signin'

/*
 * The sign-in form used to say nothing at all about a field somebody had left
 * blank. Both fields carried `required`, so the browser refused the submit
 * with its own bubble — one field at a time, gone on the next click, absent
 * from the page and so absent from a screen reader. Press the button on an
 * empty form and the most likely outcome was pressing it again.
 *
 * These are the sentences it says instead. They are pure functions because
 * the rules are worth stating, and the interesting ones are the rules that
 * deliberately do *not* fire.
 */

test('an empty address is named as empty, not as malformed', () => {
  expect(emailProblem('')).toBe('Enter your email address.')
  expect(emailProblem('   ')).toBe('Enter your email address.')
})

test('something that is not an address says so', () => {
  expect(emailProblem('dewi')).toBe('That does not look like an email address.')
  expect(emailProblem('dewi@acme')).toBe('That does not look like an email address.')
  expect(emailProblem('dewi acme.example')).toBe('That does not look like an email address.')
})

test('an address passes, and surrounding space is not the person’s mistake', () => {
  expect(emailProblem('dewi@acme.example')).toBeNull()
  expect(emailProblem('  dewi@acme.example  ')).toBeNull()
  expect(emailProblem('dewi+reports@acme.co.uk')).toBeNull()
})

test('an empty password is named', () => {
  expect(passwordProblem('')).toBe('Enter your password.')
})

/*
 * The two that matter. A length rule would have refused both of these in the
 * browser, so the server would never have seen a correct password and the
 * person holding it would have been told their own password was too short.
 */
test('a short password is not refused here — the server decides', () => {
  expect(passwordProblem('x')).toBeNull()
})

test('a password that is only spaces is a password', () => {
  expect(passwordProblem(' ')).toBeNull()
  expect(passwordProblem('  pw  ')).toBeNull()
})

test('an empty second factor is named', () => {
  expect(codeProblem('')).toBe('Enter the code from your authenticator app.')
  expect(codeProblem('  ')).toBe('Enter the code from your authenticator app.')
})

test('a recovery code is accepted alongside six digits', () => {
  expect(codeProblem('123456')).toBeNull()
  expect(codeProblem('abcd-efgh')).toBeNull()
})
