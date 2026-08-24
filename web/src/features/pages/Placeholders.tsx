import { sectionTitle, type SectionId } from "../../nav";
import AuditPage from "../audit/AuditPage";
import OverviewPage from "../overview/OverviewPage";
import DatabasesPage from "../postgres/DatabasesPage";

type PageProps = {
  section: SectionId;
  focusDatabase?: string | null;
};

export function SectionPage({ section, focusDatabase = null }: PageProps) {
  const title = sectionTitle(section);
  if (section === "overview") {
    return <OverviewPage />;
  }

  if (section === "postgres") {
    return <DatabasesPage focusDatabase={focusDatabase} />;
  }

  if (section === "audit") {
    return <AuditPage />;
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
