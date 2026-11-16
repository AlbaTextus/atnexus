"use client"

import { useState, type FormEvent } from "react"
import { Save } from "lucide-react"
import { api } from "@/lib/api"
import { managementError } from "@/lib/applications"
import { feedback } from "@/lib/feedback"
import { useAccountUser, useUpdateAccountUser, type AccountUser } from "@/components/account-shell"
import { MarkdownContent } from "@/components/markdown-content"
import { Button } from "@/components/ui/button"
import { Card, CardHeader, CardTitle, CardContent, CardFooter } from "@/components/ui/card"
import { Field, FieldLabel, FieldDescription } from "@/components/ui/field"
import { Textarea } from "@/components/ui/textarea"
import { Spinner } from "@/components/ui/spinner"

export function BioEditor() {
 const user=useAccountUser()
 const updateUser=useUpdateAccountUser()
 const [bio,setBio]=useState(user.bio || "")
 const [busy,setBusy]=useState(false)
 const dirty=bio.trim()!==(user.bio || "")
 const length=Array.from(bio).length
 async function save(event: FormEvent) {
  event.preventDefault();if(busy || !dirty || length>3000)return
  setBusy(true)
  try { const updated=await api<AccountUser>("/api/v1/account/bio",{method:"PATCH",body:JSON.stringify({bio})});updateUser(updated);setBio(updated.bio || "");feedback.success("简介已保存") }
  catch(reason){feedback.error(managementError(reason))}finally{setBusy(false)}
 }
 return <form onSubmit={save} aria-busy={busy}><Card>
  <CardHeader><CardTitle>个人简介</CardTitle></CardHeader>
  <CardContent className="flex flex-col gap-5">
   <Field data-disabled={busy} data-invalid={length>3000}><FieldLabel htmlFor="profile-bio">简介内容</FieldLabel><Textarea id="profile-bio" value={bio} disabled={busy} aria-invalid={length>3000} aria-describedby="profile-bio-help" onChange={event=>setBio(event.target.value)} className="min-h-48" placeholder={'## 关于我\n\n喜欢阅读、创作和分享。'} /><FieldDescription id="profile-bio-help">支持 Markdown 标题、列表、链接、引用与代码。{length} / 3000 字符，清空后保存可移除简介。</FieldDescription></Field>
   <section aria-label="简介预览" className="profile-bio-preview"><h3 className="mb-3 text-sm font-medium">预览</h3>{bio.trim()?<MarkdownContent content={bio}/>:<p className="text-sm text-muted-foreground">写下内容后，这里会实时显示排版效果。</p>}</section>
  </CardContent>
  <CardFooter className="gap-3"><Button type="submit" disabled={busy || !dirty || length>3000}>{busy?<Spinner data-icon="inline-start"/>:<Save data-icon="inline-start"/>}保存简介</Button><Button type="button" variant="outline" disabled={busy || !dirty} onClick={()=>setBio(user.bio || "")}>取消修改</Button></CardFooter>
 </Card></form>
}
