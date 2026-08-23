import { sectionTitle, type SectionId } from "../../nav";

type PageProps = {
  section: SectionId;
};

export function SectionPage({ section }: PageProps) {
  const title = sectionTitle(section);
  if (section === "overview") {
    return (
      <article>
        <header className="page-header">
          <h1>{title}</h1>
          <p>Independent component status. Adapters are not connected in this release slice.</p>
        </header>
        <ul className="status-cards">
          {["Redgres state", "PostgreSQL direct", "PgBouncer", "Redis"].map((name) => (
            <li key={name} className="status-card">
              <h2>{name}</h2>
              <p className="not-connected">
                <span className="warning-mark" aria-hidden="true">
                  !
                </span>
                Not connected
              </p>
            </li>
          ))}
        </ul>
      </article>
    );
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
