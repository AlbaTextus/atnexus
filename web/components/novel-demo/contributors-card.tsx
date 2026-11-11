import { Avatar } from "radix-ui";
import { History, UsersRound } from "lucide-react";
import { Link } from "next-view-transitions";
import { contentDate, safeContentURL } from "@/lib/athub-client";

type Contributor = {
  id: string;
  name: string;
  avatar: string;
};

export function ContributorAvatar({
  contributor,
  size = "default",
}: {
  contributor?: Contributor | null;
  size?: "default" | "small";
}) {
  return (
    <Avatar.Root
      className="relative flex size-9 shrink-0 overflow-hidden rounded-full data-[size=small]:size-7"
      data-size={size}
      title={contributor?.name}
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
}

export function ContributorsCard({ novelId, creator, contributors = [] }: {
  novelId: string;
  creator?: (Contributor & { createdAt?: string }) | null;
  contributors?: Contributor[];
}) {
  return (
    <section className="panel contributors-card" aria-labelledby="contributors-heading">
      <div className="sidebar-heading"><h2 id="contributors-heading"><UsersRound aria-hidden="true"/>贡献者</h2><Link href={`/library/books/${novelId}/history`} aria-label="查看变更历史" title="变更历史"><History className="size-4" aria-hidden="true"/></Link></div>
      <div className="contribution-entry">
        <ContributorAvatar contributor={creator} />
        <div className="contribution-content">
          <strong className="contributor-name">{creator?.name || "待补充"}</strong>
          <div className="contribution-activity text-muted-foreground">
            <span>创建条目 · {creator?.createdAt ? contentDate(creator.createdAt) : "待补充"}</span>
          </div>
        </div>
      </div>
      <div className="contributors-footer">
        <span>其他贡献者</span>
        <ul className="contributors-avatars" aria-label="后续贡献者">
        {contributors.map((contributor) => (
          <li key={contributor.id}>
            <ContributorAvatar contributor={contributor} size="small" />
          </li>
        ))}
        </ul>
      </div>
    </section>
  );
}
