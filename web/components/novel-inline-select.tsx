"use client"

import { Select } from "@base-ui/react/select"
import { Check, ChevronDown } from "lucide-react"

export function NovelInlineSelect({ label, value, items, disabled, onValueChange }: {
  label: string
  value: string
  items: { value: string; label: string }[]
  disabled: boolean
  onValueChange: (value: string) => void
}) {
  return <Select.Root value={value} items={items} disabled={disabled}
    onValueChange={next => { if (next !== null) onValueChange(next) }}>
    <Select.Trigger aria-label={label} className="novel-inline-select-trigger">
      <Select.Value className="novel-inline-select-value" />
      <Select.Icon className="novel-inline-select-icon"><ChevronDown size={16} aria-hidden="true" /></Select.Icon>
    </Select.Trigger>
    <Select.Portal className="novel-inline-select-portal" style={{ zoom: "calc(1 / var(--site-root-scale, var(--site-scale, 1)))" }}>
      <Select.Positioner sideOffset={6} align="start" alignItemWithTrigger={false} className="novel-inline-select-positioner">
        <Select.Popup className="novel-inline-select-popup" style={{ zoom: "var(--site-scale, 1)" }}
          onKeyDown={event => {
            // Escape dismisses the menu first; a second Escape cancels the field.
            if (event.key === "Escape") event.stopPropagation()
          }}>
          <Select.List className="novel-inline-select-list">
            {items.map(item => <Select.Item key={item.value} value={item.value} className="novel-inline-select-item">
              <Select.ItemText className="novel-inline-select-value">{item.label}</Select.ItemText>
              <Select.ItemIndicator className="novel-inline-select-indicator"><Check size={15} aria-hidden="true" /></Select.ItemIndicator>
            </Select.Item>)}
          </Select.List>
        </Select.Popup>
      </Select.Positioner>
    </Select.Portal>
  </Select.Root>
}
