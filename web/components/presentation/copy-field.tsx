"use client"

import { useEffect, useId, useRef, useState } from "react"
import { Check, Copy } from "lucide-react"
import { Field, FieldLabel } from "@/components/ui/field"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput, InputGroupPassword } from "@/components/ui/input-group"

export type CopyFieldProps = {
  label: string
  value: string
  secret?: boolean
  onCopy?: () => void
  onCopyError?: (error: unknown) => void
}

export function CopyField({ label, value, secret = false, onCopy, onCopyError }: CopyFieldProps) {
  const id = useId()
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState("")
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  useEffect(() => () => clearTimeout(timer.current), [])
  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
    } catch (cause) {
      setError("复制失败，请选中内容手动复制。")
      onCopyError?.(cause)
      return
    }
    setCopied(true)
    setError("")
    clearTimeout(timer.current)
    timer.current = setTimeout(() => setCopied(false), 2000)
    onCopy?.()
  }
  const inputProps = { id, value, readOnly: true, "aria-label": label, onFocus: (event: React.FocusEvent<HTMLInputElement>) => event.currentTarget.select() }
  const copyButton = <InputGroupButton size="icon-sm" aria-label={`复制 ${label}`} onClick={copy}>{copied ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}</InputGroupButton>
  return <Field>
    <FieldLabel htmlFor={id}>{label}</FieldLabel>
    {secret ? <InputGroupPassword {...inputProps} label="密钥">{copyButton}</InputGroupPassword>
      : <InputGroup className="h-11"><InputGroupInput {...inputProps} /><InputGroupAddon align="inline-end">{copyButton}</InputGroupAddon></InputGroup>}
    <span className="sr-only" role="status">{copied ? `${label} 已复制` : ""}</span>
    {error && <p className="text-xs text-destructive-foreground" role="alert">{error}</p>}
  </Field>
}
