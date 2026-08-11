import { useNavigate } from '@tanstack/react-router'
import { ReportForm } from '../forms/ReportForm'

/**
 * No PageHeader: the editor is a full-height workspace with its own toolbar,
 * and the shell's breadcrumb already says where you are. A page title here
 * would cost 90px of canvas to repeat something on screen twice already.
 */
export function NewReportPage() {
  const navigate = useNavigate()
  const back = () => void navigate({ to: '/' })
  return <ReportForm onDone={back} onCancel={back} />
}
