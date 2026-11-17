"use client"

import { CopyField as SharedCopyField, type CopyFieldProps } from "@/components/presentation/copy-field"
import { feedback } from "@/lib/feedback"

export function CopyField(props: Pick<CopyFieldProps, "label" | "value" | "secret">) {
  return <SharedCopyField {...props} onCopy={() => feedback.success("已复制")} onCopyError={() => feedback.error("复制失败，请选中内容手动复制。")} />
}
