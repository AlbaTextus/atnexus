import { Building2 } from "lucide-react";
import { Badge } from "./ui/badge";
import { NovelEditable } from "@/components/novel/novel-inline-editor";
import publisherBranding from "@/config/publisher-branding.json";

type PublicationCardProps = {
  releaseDate?: string;
  publisher?: string;
};

export function PublicationCard({ releaseDate, publisher }: PublicationCardProps) {
  const branding = publisherBranding.find(platform => platform.name === publisher?.trim());
  return (
    <section className="panel publication-card" aria-labelledby="publication-heading">
      <div className="sidebar-heading">
        <h2 id="publication-heading"><Building2 aria-hidden="true"/>发行信息</h2>
        {branding && <img className="publication-platform-logo" src={branding.logo} alt="" width={28} height={28}/>}
      </div>
      <dl className="publication-details" aria-label="发行信息">
        <div className="publication-producer">
          <dt>发行方</dt>
          <dd><NovelEditable field="platformId">{publisher || "待补充"}</NovelEditable></dd>
        </div>
        <div>
          <dt>发行时间</dt>
          <dd>
            {releaseDate ? (
              <time dateTime={releaseDate}>{releaseDate}</time>
            ) : (
              "待补充"
            )}
          </dd>
        </div>
        <div>
          <dt>年龄限制</dt>
          <dd className="publication-age"><Badge variant="secondary">待补充</Badge></dd>
        </div>
      </dl>
    </section>
  );
}
