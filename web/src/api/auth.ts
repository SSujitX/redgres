import { apiRequest, errorMessage, type ApiErrorBody } from "./client";

export type SessionPayload = {
  owner?: { username?: string };
  csrf_token?: string;
};

export type LoginSuccess = SessionPayload;

export async function fetchSession() {
  return apiRequest<SessionPayload & ApiErrorBody>("/api/v1/session");
}

export async function login(username: string, password: string) {
  return apiRequest<LoginSuccess & ApiErrorBody>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  });
}

export async function logout(csrf: string) {
  return apiRequest<ApiErrorBody>("/api/v1/auth/logout", {
    method: "POST",
    csrf,
  });
}

export { errorMessage };
