import type { ExpertToolsStatus, ToolLinks } from "../../api/auth";
import ExpertToolsSection from "./ExpertToolsSection";

export default function ExpertToolsPage({
  csrf,
  toolLinks,
  expertTools = {},
}: {
  csrf: string;
  toolLinks: ToolLinks;
  expertTools?: ExpertToolsStatus;
}) {
  return (
    <article className="expert-tools-page">
      <header className="page-header">
        <h1>Expert tools</h1>
        <p>
          Open pgAdmin and Redis Insight from this signed-in console. Reveal shows the saved pgAdmin email, login
          password, and master password.
        </p>
      </header>
      <ExpertToolsSection csrf={csrf} toolLinks={toolLinks} expertTools={expertTools} variant="full" />
    </article>
  );
}
