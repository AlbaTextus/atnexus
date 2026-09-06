"use client"

import { useState, type ComponentProps } from "react"
import { Eye, EyeOff } from "lucide-react"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"

type PasswordInputProps = Omit<ComponentProps<"input">, "type"> & { label?: string }

export function PasswordInput({ label = "密码", ...props }: PasswordInputProps) {
  const [visible, setVisible] = useState(false)

  return (
    <InputGroup className="h-11" aria-label={label}>
      <InputGroupInput {...props} type={visible ? "text" : "password"} className="h-full px-3" />
      <InputGroupAddon align="inline-end">
        <InputGroupButton
          size="icon-sm" className="size-10" variant="ghost"
          aria-label={`${visible ? "隐藏" : "显示"}${label}`} aria-pressed={visible}
          aria-controls={props.id} disabled={props.disabled}
          onClick={() => setVisible((value) => !value)}
        >
          {visible ? <EyeOff aria-hidden="true" /> : <Eye aria-hidden="true" />}
        </InputGroupButton>
      </InputGroupAddon>
    </InputGroup>
  )
}
