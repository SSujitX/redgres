import { apiRequest, errorMessage, type ApiErrorBody } from "./client";

export type DomainHostnames = {
  console?: string;
  db?: string;
  rs?: string;
  pgadmin?: string;
  redis?: string;
};

export type DomainActivityStep = {
  id?: string;
  label?: string;
  state?: "pending" | "running" | "done" | "failed" | string;
};

export type DomainActivity = {
  operation?: string;
  in_progress?: boolean;
  steps?: DomainActivityStep[];
};

export type DomainStatusPayload = {
  configured?: boolean;
  zone?: string;
  hostname?: string;
  hostnames?: DomainHostnames;
  origin_ip?: string;
  instructions?: string[];
  access?: string;
  tls?: Record<string, string>;
  credential?: "api_token" | "oauth" | "none";
  dns_provider?: string;
  bootstrap_still_open?: boolean;
  tunnel_id?: string;
  activity?: DomainActivity;
  request_id?: string;
};

export type DomainApplyPayload = {
  zone?: string;
  hostname?: string;
  hostnames?: DomainHostnames;
  origin_ip?: string;
  tunnel_id?: string;
  instructions?: string[];
  dns_provider?: string;
  bootstrap_still_open?: boolean;
  access?: string;
  tls?: Record<string, string>;
  request_id?: string;
};

export type DomainOkPayload = {
  ok?: boolean;
  access?: string;
  authorize_url?: string;
  redirect_uri?: string;
  scopes?: string[];
  tls?: Record<string, string>;
  bootstrap_still_open?: boolean;
  bootstrap_closed?: boolean;
  bootstrap_ufw_removed?: boolean;
  bootstrap_ufw_attempted?: boolean;
  request_id?: string;
};

export type DomainApplyInput = {
  zone: string;
  originIP: string;
  hostnames: DomainHostnames;
  dnsProvider?: "cloudflare" | "manual";
};

export async function fetchDomain(init: RequestInit = {}) {
  return apiRequest<DomainStatusPayload & ApiErrorBody>("/api/v1/domain", init);
}

export async function setDomainToken(token: string, csrf: string, init: RequestInit = {}) {
  return apiRequest<DomainOkPayload & ApiErrorBody>("/api/v1/domain/token", {
    ...init,
    method: "POST",
    csrf,
    body: JSON.stringify({ token }),
  });
}

export async function applyDomain(input: DomainApplyInput, csrf: string, init: RequestInit = {}) {
  const body: Record<string, unknown> = {
    zone: input.zone,
    origin_ip: input.originIP,
    hostnames: input.hostnames,
  };
  if (input.dnsProvider === "manual") {
    body.dns_provider = "manual";
  }
  return apiRequest<DomainApplyPayload & ApiErrorBody>("/api/v1/domain/apply", {
    ...init,
    method: "POST",
    csrf,
    body: JSON.stringify(body),
  });
}

export async function setDomainAccessPolicy(emails: string[], csrf: string, init: RequestInit = {}) {
  return apiRequest<DomainOkPayload & ApiErrorBody>("/api/v1/domain/access-policy", {
    ...init,
    method: "POST",
    csrf,
    body: JSON.stringify({ emails }),
  });
}

export async function setDomainOAuthClient(clientID: string, clientSecret: string, csrf: string, init: RequestInit = {}) {
  return apiRequest<DomainOkPayload & ApiErrorBody>("/api/v1/domain/oauth-client", {
    ...init,
    method: "POST",
    csrf,
    body: JSON.stringify({ client_id: clientID, client_secret: clientSecret }),
  });
}

export async function startDomainOAuth(csrf: string, init: RequestInit = {}) {
  return apiRequest<DomainOkPayload & ApiErrorBody>("/api/v1/domain/oauth/start", {
    ...init,
    method: "POST",
    csrf,
  });
}

export async function issueDomainTLS(csrf: string, init: RequestInit = {}) {
  return apiRequest<DomainOkPayload & ApiErrorBody>("/api/v1/domain/tls/issue", {
    ...init,
    method: "POST",
    csrf,
  });
}

export async function verifyDomainManual(csrf: string, init: RequestInit = {}) {
  return apiRequest<DomainOkPayload & { results?: Record<string, string> } & ApiErrorBody>(
    "/api/v1/domain/manual/verify",
    {
      ...init,
      method: "POST",
      csrf,
    },
  );
}

export async function confirmDomainManualAccess(csrf: string, init: RequestInit = {}) {
  return apiRequest<DomainOkPayload & ApiErrorBody>("/api/v1/domain/manual/confirm-access", {
    ...init,
    method: "POST",
    csrf,
  });
}

export async function confirmDomainReachable(csrf: string, init: RequestInit = {}) {
  return apiRequest<DomainOkPayload & ApiErrorBody>("/api/v1/domain/confirm-reachable", {
    ...init,
    method: "POST",
    csrf,
  });
}

export async function disconnectDomain(csrf: string, init: RequestInit = {}) {
  return apiRequest<DomainOkPayload & ApiErrorBody>("/api/v1/domain", {
    ...init,
    method: "DELETE",
    csrf,
  });
}

export { errorMessage };
