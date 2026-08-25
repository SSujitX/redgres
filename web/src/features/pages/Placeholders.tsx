import { sectionTitle, type SectionId } from "../../nav";
import AuditPage from "../audit/AuditPage";
import OverviewPage from "../overview/OverviewPage";
import DatabasesPage from "../postgres/DatabasesPage";
import SecurityOverview from "../postgres/SecurityOverview";
import AclUsersPage from "../redis/AclUsersPage";

type PageProps = {
  section: SectionId;
  csrf?: string;
  focusDatabase?: string | null;
  focusUsername?: string | null;
  focusNonce?: number;
};

export function SectionPage({
  section,
  csrf = "",
  focusDatabase = null,
  focusUsername = null,
  focusNonce = 0,
}: PageProps) {
  const title = sectionTitle(section);
  if (section === "overview") {
    return <OverviewPage />;
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

  const adapter =
    section.startsWith("postgres") || section.startsWith("redis")
      ? "This adapter is not available yet."
      : "This view is not available yet.";

  return (
    <article>
      <header className="page-header">
        <h1>{title}</h1>
        <p>{adapter}</p>
      </header>
    </article>
  );
}
