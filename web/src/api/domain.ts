import { apiRequest, errorMessage, type ApiErrorBody } from "./client";

export type DomainStatusPayload = {
  configured?: boolean;
  zone?: string;
  hostname?: string;
  access?: string;
  bootstrap_still_open?: boolean;
  request_id?: string;
};

export type DomainApplyPayload = {
  zone?: string;
  hostname?: string;
  tunnel_id?: string;
  bootstrap_still_open?: boolean;
  access?: string;
  request_id?: string;
};

export type DomainOkPayload = {
  ok?: boolean;
  access?: string;
  bootstrap_still_open?: boolean;
  bootstrap_closed?: boolean;
  request_id?: string;
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

export async function applyDomain(zone: string, hostname: string, csrf: string, init: RequestInit = {}) {
  return apiRequest<DomainApplyPayload & ApiErrorBody>("/api/v1/domain/apply", {
    ...init,
    method: "POST",
    csrf,
    body: JSON.stringify({ zone, hostname }),
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
