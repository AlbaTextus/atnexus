"use client";
import { useState, type ReactNode } from "react";
import { Link } from "next-view-transitions";
import { NovelCover } from "./novel-cover";
import { MotionConfig } from "framer-motion";
import { Archive, BookOpenText, CircleCheck, CircleEllipsis, ListTree, Tags, UserRound } from "lucide-react";
import { Badge } from "./ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "./ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

import { PublicationCard } from "./publication-card";
import { ContributorsCard } from "./contributors-card";
import { GallerySection } from "./gallery-section";
import { ChapterTree } from "./chapter-tree";
import { AuthorSection } from "./author-section";
import { ContentSectionHeading } from "./content-section-heading";
import type { NovelDetailModel } from "@/lib/novel/novel-view-models";
import { NovelEditable, NovelEditToolbar, useNovelEditor, useNovelPending } from "@/components/novel/novel-inline-editor";
import { NovelPendingChanges } from "@/components/novel/novel-pending-changes";

export function NovelDetailView({novel: savedNovel,catalogLoading=false,catalogError="",actions}: {novel: NovelDetailModel;catalogLoading?:boolean;catalogError?:string;actions?:ReactNode}) {
  const editor = useNovelEditor();
  // A novel without tags has no tag panel, unless it can be given some or a submission awaiting review gives it some.
  const tagsPending = useNovelPending("tagIds").length > 0;
  const summaryPending = useNovelPending("summary").length > 0;
  const novel = editor?.preview ? {...savedNovel, ...editor.preview} : savedNovel;
  const [cover,setCover]=useState(false);
  const [failedBackdrop,setFailedBackdrop]=useState<string | null>(null);
  const statusAppearance = {
    "已完结": { icon: CircleCheck, color: "text-emerald-500" },
    "连载中": { icon: CircleEllipsis, color: "text-yellow-500" },
    "太监": { icon: CircleEllipsis, color: "text-red-500" },
  }[novel.status];
  const StatusIcon = statusAppearance?.icon;
  const facts=[["总字数",novel.words],["章节数量",novel.chapterCount === null ? "待补充" : `${novel.chapterCount} 章`],["最后更新",novel.updatedAt.slice(0,10)]];
  return <MotionConfig reducedMotion="user">
  <main className="page-shell novel-detail-page novel-content-ready">
  <NovelEditToolbar/>
  <section id="overview" className="novel-book-header" aria-labelledby="novel-title">
    {novel.cover && failedBackdrop !== novel.cover && <div className="novel-cover-ambience" aria-hidden="true">
      <img src={novel.cover} alt="" decoding="async" referrerPolicy="no-referrer" onError={()=>setFailedBackdrop(novel.cover)}/>
    </div>}
    <div className="cover-area"><NovelEditable field="coverUrl" block viewAction={()=>setCover(true)}><button className="cover-button" onClick={()=>setCover(true)} aria-label="查看完整封面"><NovelCover src={novel.cover} title={novel.title}/></button></NovelEditable><NovelPendingChanges field="coverUrl"/></div>
    <div className="novel-book-heading">
      <h1 id="novel-title"><NovelEditable field="title">{novel.title}</NovelEditable></h1>
      <NovelPendingChanges field="title"/>
      <div className="novel-book-author"><span className="novel-author-role"><UserRound aria-hidden="true"/>作者</span><span className="author-name"><NovelEditable field="authorId">{novel.authorId === null ? novel.author : <Link className="novel-author-link" href={`/library?authorId=${novel.authorId}`}>{novel.author}</Link>}</NovelEditable></span></div>
      <NovelPendingChanges field="authorId"/>
    </div>
  </section>
  <hr className="novel-book-divider"/>
  <div className="novel-details-body"><div className="details-grid" id="details"><aside className="sidebar">
  {(novel.tags.length > 0 || editor?.mode && (editor.allowed || tagsPending)) && <section className="panel book-tags" aria-labelledby="book-tags-heading">
    <div className="sidebar-heading"><h2 id="book-tags-heading"><Tags aria-hidden="true"/>标签</h2></div>
    <NovelEditable field="tagIds" block><span className="tag-cloud">{novel.tags.length ? novel.tags.map(t=><Badge variant="secondary" key={t}>{t}</Badge>) : editor?.allowed ? "添加标签" : "暂无标签"}</span></NovelEditable>
    <NovelPendingChanges field="tagIds"/>
  </section>}
  <section className="panel book-information" aria-labelledby="book-information-heading">
    <div className="sidebar-heading"><h2 id="book-information-heading"><Archive aria-hidden="true"/>作品档案</h2><NovelEditable field="status">{StatusIcon ? <Tooltip><TooltipTrigger render={<span tabIndex={0} />} className={`inline-flex shrink-0 rounded-full outline-offset-4 focus-visible:outline-2 focus-visible:outline-ring ${statusAppearance.color}`} aria-label={novel.status}><StatusIcon className="size-5" aria-hidden="true"/></TooltipTrigger><TooltipContent>{novel.status}</TooltipContent></Tooltip> : <Badge variant="secondary">{novel.status}</Badge>}</NovelEditable></div>
    <NovelPendingChanges field="status"/>
    <dl className="archive-facts">{facts.map(([k,v])=><div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}</dl>
  </section>
  <PublicationCard publisher={novel.publisher}/>
  <ContributorsCard novelId={novel.id} creator={novel.contributions?.creator} contributors={novel.contributions?.contributors} actions={actions}/>
  </aside>
  <div className="main-column">
    {(novel.synopsis.length > 0 || summaryPending || editor?.mode && editor.allowed) && <section className="panel introduction" aria-label="作品简介"><ContentSectionHeading icon={BookOpenText}>简介</ContentSectionHeading><div className="synopsis"><NovelEditable field="summary" block>{novel.synopsis.length ? novel.synopsis.map((p,i)=><p key={i}>{p}</p>) : "暂无简介"}</NovelEditable></div><NovelPendingChanges field="summary" current={savedNovel.rawSummary ?? ""}/></section>}
    <GallerySection novelId={Number(novel.id)}/>
  </div>
    {catalogLoading || catalogError ? <section className="panel novel-chapter-catalog" aria-labelledby="novel-catalog-heading">
      <ContentSectionHeading id="novel-catalog-heading" icon={ListTree} className="catalog-heading">目录</ContentSectionHeading>
      {catalogLoading ? <div className="novel-page-loading" role="status">正在加载目录…</div> : <div role="alert">{catalogError}</div>}
    </section> : <ChapterTree volumes={novel.volumes} chapters={novel.chapters}/>}
    <AuthorSection key={`${novel.id}:${novel.authorId}`} novelId={novel.id} authorId={novel.authorId}/>
  </div></div>
  </main>
  <Dialog open={cover} onOpenChange={setCover}><DialogContent><DialogHeader><DialogTitle>作品封面</DialogTitle><DialogDescription>{novel.title}</DialogDescription></DialogHeader><NovelCover src={novel.cover} title={novel.title} full/></DialogContent></Dialog></MotionConfig>;
}
