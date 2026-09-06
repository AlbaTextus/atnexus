"use client"

import { useId, useState } from "react"
import { Check, Copy, Eye, EyeOff } from "lucide-react"
import { Field, FieldLabel } from "@/components/ui/field"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"

export function CopyField({ label, value, secret = false }: { label: string; value: string; secret?: boolean }) {
  const id = useId()
  const [visible, setVisible] = useState(false)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState("")
  async function copy() {
    try { await navigator.clipboard.writeText(value); setCopied(true); setError(""); setTimeout(() => setCopied(false), 2000) }
    catch { setError("复制失败，请选中内容手动复制。") }
  }
  return <Field><FieldLabel htmlFor={id}>{label}</FieldLabel><InputGroup className="h-11"><InputGroupInput id={id} type={secret && !visible ? "password" : "text"} value={value} readOnly aria-label={label} onFocus={event => event.currentTarget.select()} /><InputGroupAddon align="inline-end">{secret && <InputGroupButton size="icon-sm" aria-label={visible ? "隐藏密钥" : "显示密钥"} onClick={() => setVisible(!visible)}>{visible ? <EyeOff /> : <Eye />}</InputGroupButton>}<InputGroupButton size="icon-sm" aria-label={`复制 ${label}`} onClick={copy}>{copied ? <Check /> : <Copy />}</InputGroupButton></InputGroupAddon></InputGroup><span className="sr-only" role="status">{copied ? `${label} 已复制` : ""}</span>{error && <p className="text-xs text-destructive" role="alert">{error}</p>}</Field>
}
