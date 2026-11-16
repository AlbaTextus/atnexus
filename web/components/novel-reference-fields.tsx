"use client"

import { useId, useRef } from "react"
import { X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import type { ContentAuthor, ContentTag } from "@/lib/athub-client"

export function NovelAuthorField({ value, authors, disabled, onChange }: {
  value: string; authors: ContentAuthor[]; disabled: boolean; onChange: (value: string) => void;
}) {
  const suggestions = useId()
  const names = [...new Set(authors.map(author => author.name?.trim()).filter((name): name is string => Boolean(name)))]
  return <span className="novel-inline-author">
    <Input aria-label="作者" placeholder="填写作者名字" value={value} disabled={disabled}
      list={suggestions} autoComplete="off" onChange={event => onChange(event.target.value)} />
    <datalist id={suggestions}>{names.map(name => <option key={name} value={name} />)}</datalist>
  </span>
}

export function NovelTagsField({ values, tags, disabled, onChange }: {
  values: string[]; tags: ContentTag[]; disabled: boolean; onChange: (values: string[]) => void;
}) {
  const host = useRef<HTMLSpanElement>(null)
  const selected = new Set(values.map(value => value.trim()).filter(Boolean))
  const suggestions = tags.filter(tag => !selected.has(tag.name.trim()))
  return <span ref={host} className="novel-inline-tag-editor" role="group" aria-label="标签"
    onKeyDown={event => {
      if (event.key !== "Enter" || !(event.target instanceof HTMLInputElement)) return
      event.stopPropagation()
      if (event.nativeEvent.isComposing) return
      event.preventDefault()
      host.current?.querySelector<HTMLInputElement>("[data-new-tag]")?.focus()
    }}>
    <span className="novel-inline-tag-values">
      {[...values, ""].map((value, index) => <span className={index === values.length ? "novel-inline-tag-add" : "novel-inline-tag-entry"} key={index}>
        <Input aria-label={index === values.length ? "新增标签" : `第 ${index + 1} 个标签`} value={value} disabled={disabled}
          data-new-tag={index === values.length ? "" : undefined}
          placeholder={index === values.length ? "添加标签" : undefined}
          className={index === values.length ? "novel-inline-tag-new" : undefined}
          style={index === values.length ? undefined : { width: `calc(${Math.max(2, Array.from(value).length)}em + 16px)` }}
          onChange={event => onChange(index === values.length
            ? [...values, event.target.value] : values.map((name, i) => i === index ? event.target.value : name))} />
        {index < values.length && <Button type="button" variant="ghost" size="icon-sm" aria-label={`移除标签${value.trim() || index + 1}`}
          disabled={disabled} onClick={() => onChange(values.filter((_, i) => i !== index))}>
          <X className="size-3.5" aria-hidden="true" />
        </Button>}
      </span>)}
    </span>
    {suggestions.length > 0 && <span className="novel-inline-tags" aria-label="已有标签">
      {suggestions.map(tag => <Button type="button" key={tag.id} size="sm" variant="outline" disabled={disabled}
        onClick={() => onChange([...values, tag.name])}>{tag.name}</Button>)}
    </span>}
  </span>
}
