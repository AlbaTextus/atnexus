"use client";
import { useState } from "react";
import { Link } from "next-view-transitions";
import { DemoCover } from "./novel-cover";
import { MotionConfig } from "framer-motion";
import { Archive, BookOpen, Download, FileText, Image, Library, Tags, UserRound } from "lucide-react";
import { Badge } from "./ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./ui/tabs";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "./ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

import { PublicationCard } from "./publication-card";
import { ContributorsCard } from "./contributors-card";
import { GallerySection } from "./gallery-section";
import { DownloadSection } from "./download-section";
import { ChapterTree } from "./chapter-tree";
import type { DemoNovel } from "@/lib/novel-demo-data";

const navigation = [{href:"/library",title:"书库",icon:Library},{href:"#overview",title:"作品概览",icon:BookOpen},{href:"#details",title:"作品详情",icon:Library}];
export function NovelDetail({novel,catalogLoading=false,catalogError=""}: {novel: DemoNovel;catalogLoading?:boolean;catalogError?:string}) {
  const [tab,setTab]=useState("intro"), [cover,setCover]=useState(false);
  const facts=[["总字数",novel.words],["章节数量",novel.chapterCount === null ? "待补充" : `${novel.chapterCount} 章`],["作品语言",novel.language],["最后更新",novel.updatedAt.slice(0,10)]];
  return <MotionConfig reducedMotion="user">
  <main className="page-shell novel-detail-page novel-content-ready">
  <div className="details-grid" id="details"><aside className="sidebar">
  <section id="overview" className="panel book-overview" aria-labelledby="book-cover-heading">
    <div className="sidebar-heading"><h2 id="book-cover-heading"><Image aria-hidden="true"/>封面</h2></div>
    <div className="cover-area"><button className="cover-button" onClick={()=>setCover(true)} aria-label="查看完整封面"><DemoCover src={novel.cover} title={novel.title}/></button></div>
  </section>
  <section className="panel book-title-card" aria-labelledby="book-title-heading">
    <div className="sidebar-heading"><h2 id="book-title-heading"><BookOpen aria-hidden="true"/>作品名称</h2></div>
    <h1 id="novel-title">{novel.title}</h1>
  </section>
  <section className="panel author-card" aria-label="作者">
    <div className="sidebar-heading"><h2><UserRound aria-hidden="true"/>作者</h2></div>
    <p className="author-name">{novel.authorId === null ? novel.author : <Link className="novel-author-link" href={`/library?authorId=${novel.authorId}`}>{novel.author}</Link>}</p>
  </section>
  {novel.tags.length > 0 && <section className="panel book-tags" aria-labelledby="book-tags-heading">
    <div className="sidebar-heading"><h2 id="book-tags-heading"><Tags aria-hidden="true"/>标签</h2></div>
    <div className="tag-cloud">{novel.tags.map(t=><Badge variant="secondary" key={t}>{t}</Badge>)}</div>
  </section>}
  <section className="panel book-information" aria-labelledby="book-information-heading">
    <div className="sidebar-heading"><h2 id="book-information-heading"><Archive aria-hidden="true"/>作品档案</h2><Badge variant="secondary">{novel.status}</Badge></div>
    <dl className="archive-facts">{facts.map(([k,v])=><div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}</dl>
  </section>
  <PublicationCard publisher={novel.publisher}/>
  <ContributorsCard novelId={novel.id} creator={novel.contributions?.creator} contributors={novel.contributions?.contributors}/>
  </aside>
  <div className="main-column"><Tabs value={tab} onValueChange={setTab}>
    <div className="novel-section-navigation"><TabsList className="novel-section-tabs" aria-label="作品内容">
      <Tooltip><TabsTrigger value="intro" asChild><TooltipTrigger aria-label="作品简介"><FileText aria-hidden="true"/></TooltipTrigger></TabsTrigger><TooltipContent>作品简介</TooltipContent></Tooltip>
      <Tooltip><TabsTrigger value="catalog" data-section="catalog" asChild><TooltipTrigger aria-label="章节目录"><Library aria-hidden="true"/></TooltipTrigger></TabsTrigger><TooltipContent>章节目录</TooltipContent></Tooltip>
      <Tooltip><TabsTrigger value="downloads" data-section="downloads" asChild><TooltipTrigger aria-label="下载"><Download aria-hidden="true"/></TooltipTrigger></TabsTrigger><TooltipContent>下载</TooltipContent></Tooltip>
    </TabsList></div>
    <TabsContent value="intro"><section className="panel introduction" aria-label="作品简介"><h2 className="content-section-heading m-0 text-lg font-bold tracking-tight text-foreground sm:text-xl">简介</h2><div className="synopsis">{novel.synopsis.map((p,i)=><p key={i}>{p}</p>)}</div></section><GallerySection/></TabsContent>
    <TabsContent value="downloads"><DownloadSection/></TabsContent>
    <TabsContent value="catalog">{catalogLoading ? <div className="panel novel-page-loading" role="status">正在加载目录…</div> : catalogError ? <div className="panel" role="alert">{catalogError}</div> : <ChapterTree volumes={novel.volumes} chapters={novel.chapters}/>}</TabsContent>
  </Tabs></div></div>
  </main>
  <Dialog open={cover} onOpenChange={setCover}><DialogContent><DialogHeader><DialogTitle>作品封面</DialogTitle><DialogDescription>{novel.title}</DialogDescription></DialogHeader><DemoCover src={novel.cover} title={novel.title} full/></DialogContent></Dialog></MotionConfig>;
}
