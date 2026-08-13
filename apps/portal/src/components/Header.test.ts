import { describe, expect, test } from 'bun:test'
import { crumbs } from './Header'

/*
 * Every page says where it is.
 *
 * The breadcrumb was a map of fixed titles, which can only name the pages that
 * take no argument. Everything below a section takes one — every editor, every
 * report — so those pages read "acme / finance" and stopped, and the one
 * fallback that existed said "Report" over a page editing a specific report: a
 * label true of all of them and identifying none.
 */
describe('the trail to a page', () => {
  test('the sections name themselves', () => {
    expect(crumbs('/')).toEqual(['Reports'])
    expect(crumbs('/data')).toEqual(['Data'])
    expect(crumbs('/schedules')).toEqual(['Schedules'])
    expect(crumbs('/activity')).toEqual(['Activity'])
    expect(crumbs('/settings')).toEqual(['Settings'])
    expect(crumbs('/account')).toEqual(['Your account'])
  })

  test('a report names itself, rather than being called "Report"', () => {
    expect(crumbs('/reports/billing-summary')).toEqual(['Reports', 'billing-summary'])
  })

  test('and an editor names what it is editing', () => {
    // These four read "acme / finance" and nothing else.
    expect(crumbs('/reports/billing-summary/edit')).toEqual(['Reports', 'billing-summary'])
    expect(crumbs('/schedules/monthly-statements/edit'))
      .toEqual(['Schedules', 'monthly-statements'])
    // Sources and datasets are different things sharing one page, so the kind
    // is a crumb: "Data / warehouse" would not say which of the two it is.
    expect(crumbs('/data/sources/warehouse/edit'))
      .toEqual(['Data', 'Sources', 'warehouse'])
    expect(crumbs('/data/datasets/invoices/edit'))
      .toEqual(['Data', 'Datasets', 'invoices'])
  })

  test('the new-report page is the one below a section with no name yet', () => {
    // Because it is the page where somebody chooses one.
    expect(crumbs('/reports/new')).toEqual(['Reports', 'New report'])
  })

  test('a path nothing serves gets no trail rather than an invented one', () => {
    // A 404 should not be labelled as though it were a page.
    expect(crumbs('/nowhere')).toEqual([])
    expect(crumbs('/nowhere/deeper')).toEqual([])
  })

  test('a trailing slash is not a crumb', () => {
    expect(crumbs('/data/')).toEqual(['Data'])
    expect(crumbs('/reports/billing-summary/')).toEqual(['Reports', 'billing-summary'])
  })
})
