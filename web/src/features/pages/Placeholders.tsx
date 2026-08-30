import type { ToolLinks } from "../../api/auth";
import type { SectionId } from "../../nav";
import AuditPage from "../audit/AuditPage";
import DocsPage from "../docs/DocsPage";
import DomainNetworkPage from "../domain/DomainNetworkPage";
import OverviewPage from "../overview/OverviewPage";
import DatabasesPage from "../postgres/DatabasesPage";
import SecurityOverview from "../postgres/SecurityOverview";
import AclUsersPage from "../redis/AclUsersPage";
import PresetsPage from "../redis/PresetsPage";
import SystemPage from "../system/SystemPage";

type PageProps = {
  section: SectionId;
  csrf?: string;
  toolLinks?: ToolLinks;
  focusDatabase?: string | null;
  focusUsername?: string | null;
  focusArticle?: string | null;
  focusNonce?: number;
  onSelectArticle?: (id: string) => void;
  onBackToDocs?: () => void;
};

export function SectionPage({
  section,
  csrf = "",
  toolLinks = {},
  focusDatabase = null,
  focusUsername = null,
  focusArticle = null,
  focusNonce = 0,
  onSelectArticle = () => {},
  onBackToDocs = () => {},
}: PageProps) {
  if (section === "overview") {
    return <OverviewPage csrf={csrf} toolLinks={toolLinks} />;
  }

  if (section === "postgres" || section === "postgres-create") {
    return (
      <DatabasesPage
        csrf={csrf}
        focusDatabase={focusDatabase}
        focusNonce={focusNonce}
        openCreate={section === "postgres-create"}
      />
    );
  }

  if (section === "postgres-security") {
    return <SecurityOverview />;
  }

  if (section === "audit") {
    return <AuditPage />;
  }

  if (section === "redis") {
    return <AclUsersPage csrf={csrf} focusUsername={focusUsername} focusNonce={focusNonce} />;
  }

  if (section === "redis-presets") {
    return <PresetsPage />;
  }

  if (section === "system") {
    return <SystemPage csrf={csrf} toolLinks={toolLinks} />;
  }

  if (section === "domain") {
    return <DomainNetworkPage csrf={csrf} />;
  }

  if (section === "docs") {
    return (
      <DocsPage
        focusArticle={focusArticle}
        focusNonce={focusNonce}
        onSelectArticle={onSelectArticle}
        onBack={onBackToDocs}
      />
    );
  }

  const exhaustive: never = section;
  return exhaustive;
}
