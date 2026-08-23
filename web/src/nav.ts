export type SectionId =
  | "overview"
  | "postgres"
  | "postgres-create"
  | "postgres-security"
  | "redis"
  | "redis-presets"
  | "audit"
  | "system"
  | "docs";

export type ServiceId = "postgres" | "redis";

export type NavIconId =
  | "overview"
  | "database"
  | "plus"
  | "shield"
  | "key"
  | "sliders"
  | "audit"
  | "system"
  | "docs";

export type NavEntry = {
  id: SectionId;
  label: string;
  group: string;
  service: ServiceId | null;
  icon: NavIconId;
  nested: boolean;
};

export const navEntries: NavEntry[] = [
  { id: "overview", label: "Overview", group: "Overview", service: null, icon: "overview", nested: false },
  { id: "postgres", label: "Databases", group: "PostgreSQL", service: "postgres", icon: "database", nested: false },
  { id: "postgres-create", label: "Create database", group: "PostgreSQL", service: "postgres", icon: "plus", nested: true },
  { id: "postgres-security", label: "Security overview", group: "PostgreSQL", service: "postgres", icon: "shield", nested: true },
  { id: "redis", label: "ACL users", group: "Redis ACL", service: "redis", icon: "key", nested: false },
  { id: "redis-presets", label: "Permission presets", group: "Redis ACL", service: "redis", icon: "sliders", nested: true },
  { id: "audit", label: "Audit", group: "Audit", service: null, icon: "audit", nested: false },
  { id: "system", label: "System", group: "System", service: null, icon: "system", nested: false },
  { id: "docs", label: "Documentation", group: "Documentation", service: null, icon: "docs", nested: false },
];

export function serviceActive(section: SectionId, service: ServiceId): boolean {
  if (service === "postgres") {
    return section === "postgres" || section === "postgres-create" || section === "postgres-security";
  }
  return section === "redis" || section === "redis-presets";
}

export function visibleNavEntries(section: SectionId): NavEntry[] {
  return navEntries.filter((entry) => {
    if (!entry.nested || !entry.service) {
      return true;
    }
    return serviceActive(section, entry.service);
  });
}

export function filterNav(query: string): NavEntry[] {
  const q = query.trim().toLowerCase();
  if (q.length < 1) {
    return [];
  }
  return navEntries.filter(
    (entry) =>
      entry.label.toLowerCase().includes(q) ||
      entry.group.toLowerCase().includes(q),
  );
}

export function sectionTitle(id: SectionId): string {
  return navEntries.find((entry) => entry.id === id)?.label ?? "Overview";
}
