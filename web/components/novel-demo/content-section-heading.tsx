import type { ComponentProps } from "react";

export function ContentSectionHeading({ className = "", ...props }: ComponentProps<"h2">) {
  return <h2 {...props} className={`content-section-heading m-0 text-lg font-extrabold tracking-tight text-foreground sm:text-2xl ${className}`} />;
}
