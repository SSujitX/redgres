export const docArticleIds = ["using-search", "postgres-databases", "redis-acl-users", "credentials"] as const;

export type DocArticleId = (typeof docArticleIds)[number];

export type DocArticle = {
  id: DocArticleId;
  title: string;
  keywords: readonly string[];
};

export const docArticles: readonly DocArticle[] = [
  {
    id: "using-search",
    title: "Using search",
    keywords: ["search", "palette", "keyboard", "navigation"],
  },
  {
    id: "postgres-databases",
    title: "PostgreSQL databases",
    keywords: ["databases", "inspect", "tables", "rows"],
  },
  {
    id: "redis-acl-users",
    title: "Redis ACL users",
    keywords: ["ACL", "username", "preset", "prefix"],
  },
  {
    id: "credentials",
    title: "Passwords and tickets",
    keywords: ["credential", "ticket", "reveal", "rotate", "vault"],
  },
];

export function isDocArticleId(id: string): id is DocArticleId {
  return docArticles.some((article) => article.id === id);
}

export function lookupDoc(id: string): DocArticle | undefined {
  return docArticles.find((article) => article.id === id);
}
