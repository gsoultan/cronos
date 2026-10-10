/*
 * A report in a page with no framework and no build step.
 *
 * The element is already loaded — the script tag in index.html brought it
 * from the cronos server. All this file does is ask our own backend for a
 * token and hand it over.
 */
const cronos = document.body.dataset.cronos
const account = document.querySelector('[data-testid=account]')
const stage = document.querySelector('[data-testid=stage]')
let refresh

const api = (path, body) =>
  fetch(path, body === undefined ? {} : {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  }).then((res) => res.json())

function button(label, testid, onClick) {
  const b = document.createElement('button')
  b.textContent = label
  b.dataset.testid = testid
  b.addEventListener('click', onClick)
  return b
}

function notice(text) {
  const p = document.createElement('p')
  p.className = 'notice'
  p.dataset.testid = 'notice'
  p.textContent = text
  return p
}

/*
 * The token comes from our backend, which knows who is signed in. This page
 * never chooses whose rows it sees and could not if it tried: it does not
 * hold a key, and it cannot read or change the token it is given.
 */
async function showReport() {
  const { token, expiresAt, error } = await api('/api/report-token', {})
  if (error) return stage.replaceChildren(notice(error))

  let report = stage.querySelector('cronos-report')
  if (!report) {
    report = document.createElement('cronos-report')
    report.setAttribute('endpoint', cronos)
    report.setAttribute('report', 'billing-summary')
    // A message meant for a person: an expired link, a report that is gone.
    report.addEventListener('cronos:error', (e) => console.warn('cronos:', e.detail.message))
    stage.replaceChildren(report)
  }
  report.setAttribute('token', token)

  /* Tokens are short-lived on purpose — a token in a browser is a credential
     in somebody's devtools. Ask for the next one before this one runs out,
     so a tab left open all afternoon never shows an expired report. */
  clearTimeout(refresh)
  refresh = setTimeout(showReport, Math.max(30_000, (expiresAt - Date.now()) * 0.8))
}

async function render() {
  const { user, users } = await api('/api/session')
  clearTimeout(refresh)

  if (!user) {
    account.replaceChildren(...users.map((name) =>
      button(`Sign in as ${name}`, `sign-in-${name}`, async () => {
        await api('/api/sign-in', { user: name })
        render()
      })))
    stage.replaceChildren(notice('Sign in to see your billing summary.'))
    return
  }

  const who = document.createElement('span')
  who.dataset.testid = 'user'
  who.textContent = user
  account.replaceChildren(who, button('Sign out', 'sign-out', async () => {
    await api('/api/sign-out', {})
    render()
  }))
  await showReport()
}

render()
