"use client";

import { Avatar } from "radix-ui";
import { useEffect, useRef, useState } from "react";
import { History, MoreHorizontal, UsersRound } from "lucide-react";
import { Link } from "next-view-transitions";
import { contentDate, safeContentURL } from "@/lib/athub-client";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

type Contributor = {
  id: string;
  name: string;
  avatar: string;
};

export function ContributorAvatar({
  contributor,
  size = "default",
  tooltip = true,
}: {
  contributor?: Contributor | null;
  size?: "default" | "small";
  tooltip?: boolean;
}) {
  const avatar = (
    <Avatar.Root
      className="relative flex size-9 shrink-0 overflow-hidden rounded-full data-[size=small]:size-7"
      data-size={size}
    >
      {contributor && safeContentURL(contributor.avatar) && <Avatar.Image
        className="size-full object-cover"
        src={contributor.avatar}
        alt={`${contributor.name}的头像`}
      />}
      <Avatar.Fallback className="flex size-full items-center justify-center bg-secondary text-xs font-medium text-secondary-foreground">
        {contributor?.name.slice(0, 1).toUpperCase() || "—"}
      </Avatar.Fallback>
    </Avatar.Root>
  );
  return tooltip && contributor?.name ? <Tooltip>
    <TooltipTrigger render={avatar} tabIndex={0} aria-label={contributor.name} />
    <TooltipContent>{contributor.name}</TooltipContent>
  </Tooltip> : avatar;
}

function ContributorList({ contributors }: { contributors: Contributor[] }) {
  const listRef = useRef<HTMLUListElement>(null);
  const [capacity, setCapacity] = useState(1);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const measure = () => {
      const item = list.firstElementChild as HTMLElement | null;
      if (!item) return;
      const gap = parseFloat(getComputedStyle(list).columnGap) || 0;
      setCapacity(Math.max(1, Math.floor((list.clientWidth + gap) / (item.offsetWidth + gap))));
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(list);
    return () => observer.disconnect();
  }, []);

  const overflowing = contributors.length > capacity;
  const visible = overflowing ? contributors.slice(0, Math.max(0, capacity - 1)) : contributors;

  return <ul ref={listRef} className="contributors-avatars" aria-label="贡献者">
    {visible.map(contributor => <li key={contributor.id}>
      <ContributorAvatar contributor={contributor} size="small" />
    </li>)}
    {overflowing && <li className="contributors-more-item">
      <Tooltip open={expanded} onOpenChange={setExpanded}>
        <TooltipTrigger className="contributors-more" aria-label={`查看全部 ${contributors.length} 位贡献者`} closeOnClick={false} onClick={() => setExpanded(true)}>
          <MoreHorizontal aria-hidden="true" />
        </TooltipTrigger>
        <TooltipContent side="top" align="start" className="contributors-expanded">
          <ul aria-label="全部贡献者">
            {contributors.map(contributor => <li key={contributor.id}>
              <ContributorAvatar contributor={contributor} size="small" tooltip={false} />
              <span>{contributor.name || "待补充"}</span>
            </li>)}
          </ul>
        </TooltipContent>
      </Tooltip>
    </li>}
  </ul>;
}

export function ContributorsCard({ novelId, creator, contributors = [] }: {
  novelId: string;
  creator?: (Contributor & { createdAt?: string }) | null;
  contributors?: Contributor[];
}) {
  return (
    <section className="panel contributors-card" aria-labelledby="contributors-heading">
      <div className="sidebar-heading"><h2 id="contributors-heading"><UsersRound aria-hidden="true"/>贡献者</h2><Tooltip><TooltipTrigger render={<Link href={`/library/books/${novelId}/history`} aria-label="查看变更历史" />}><History className="size-4" aria-hidden="true"/></TooltipTrigger><TooltipContent>变更历史</TooltipContent></Tooltip></div>
      <div className="contribution-entry">
        <ContributorAvatar contributor={creator} />
        <div className="contribution-content">
          <strong className="contributor-name">{creator?.name || "待补充"}</strong>
          <div className="contribution-activity text-muted-foreground">
            <span>创建条目 · {creator?.createdAt ? contentDate(creator.createdAt) : "待补充"}</span>
          </div>
        </div>
      </div>
      {contributors.length > 0 && <div className="contributors-footer"><ContributorList contributors={contributors} /></div>}
    </section>
  );
}
