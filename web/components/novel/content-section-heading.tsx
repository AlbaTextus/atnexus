import type { ComponentProps } from "react";
import type { LucideIcon } from "lucide-react";

export function ContentSectionHeading({
  icon: Icon,
  className = "",
  children,
  ...props
}: ComponentProps<"h2"> & { icon?: LucideIcon }) {
  return (
    <h2 {...props} className={`content-section-heading m-0 text-lg font-extrabold tracking-tight text-foreground sm:text-2xl ${className}`}>
      {Icon && <Icon aria-hidden="true" />}
      {children}
    </h2>
  );
}
