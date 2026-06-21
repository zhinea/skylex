import { Card as ShadcnCard, CardHeader, CardTitle, CardAction, CardContent } from "~/components/ui/card";

export function Card({ title, children, className = "", actions }: {
  title?: string;
  children: React.ReactNode;
  className?: string;
  actions?: React.ReactNode;
}) {
  return (
    <ShadcnCard className={className}>
      {(title || actions) && (
        <CardHeader className="border-b">
          {title && <CardTitle className="text-sm font-semibold">{title}</CardTitle>}
          {actions && <CardAction>{actions}</CardAction>}
        </CardHeader>
      )}
      <CardContent className="py-4">{children}</CardContent>
    </ShadcnCard>
  );
}