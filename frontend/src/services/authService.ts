import { exchangeConfig } from './exchangeService';

export interface AuthUser {
  sub: string;
  email?: string;
  name?: string;
  roles?: string[];
  exp?: string;
}

export interface AuthStatus {
  enabled: boolean;
  provider: string;
}

export interface AuthSession {
  authenticated: boolean;
  enabled?: boolean;
  user?: AuthUser;
}

export async function fetchAuthStatus(): Promise<AuthStatus> {
  return authJSON<AuthStatus>('/auth/oidc/status');
}

export async function fetchAuthSession(): Promise<AuthSession> {
  return authJSON<AuthSession>('/auth/me');
}

export async function logout(): Promise<void> {
  await authJSON<AuthSession>('/auth/logout', { method: 'POST' });
}

export function oidcLoginURL(): string {
  const redirect = typeof window === 'undefined' ? '/' : window.location.origin + window.location.pathname;
  const query = new URLSearchParams({ redirect });
  return `${exchangeConfig.apiBaseURL}/auth/oidc/login?${query.toString()}`;
}

async function authJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${exchangeConfig.apiBaseURL}${path}`, {
    ...init,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers || {}),
    },
  });

  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    throw new Error(payload.error || `Auth API error ${response.status}`);
  }

  return response.json() as Promise<T>;
}
