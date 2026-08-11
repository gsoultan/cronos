import { Button } from '@mantine/core'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'

interface Props {
  title: string
  description: string
  emptyTitle: string
  emptyDescription: string
  cta: string
}

/** Destinations that exist in the nav but not yet in the build. */
export function PlaceholderPage({ title, description, emptyTitle, emptyDescription, cta }: Props) {
  return (
    <>
      <PageHeader title={title} description={description} actions={<Button>{cta}</Button>} />
      <EmptyState title={emptyTitle} description={emptyDescription}
        action={<Button variant="default">{cta}</Button>} />
    </>
  )
}
