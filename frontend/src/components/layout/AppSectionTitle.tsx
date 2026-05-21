import { formatHeaderBrand } from '@/lib/appBrand'

type AppSectionTitleProps = {
  hostname?: string | null
  section: string
}

export default function AppSectionTitle({
  hostname,
  section,
}: AppSectionTitleProps) {
  return (
    <>
      <span className="truncate">{formatHeaderBrand(hostname)}</span>
      <span className="text-muted-foreground">/</span>
      <span className="truncate text-muted-foreground">{section}</span>
    </>
  )
}
